package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	assessmentapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	assessmenthttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/transport/http"
	variantpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/infrastructure/postgres"
)

func TestAssessmentEndpointOwnershipHistoricalSnapshotAndStrictResults(t *testing.T) {
	f := newFixture(t)
	access, _, ownerID := f.register(t, "assessment-owner@example.edu")
	other, _, otherID := f.register(t, "assessment-other@example.edu")
	for _, tr := range []struct{ id, owner string }{{"owned-answer", ownerID}, {"foreign-answer", otherID}} {
		if _, err := f.pool.Exec(context.Background(), "INSERT INTO transcriptions(id,user_id,text,created_at) VALUES ($1,$2,$3,$4)", tr.id, tr.owner, "Ответ студента", f.now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	create := func(key string) (string, string, string) {
		req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", nil)
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(access)
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, req)
		var value struct {
			ID           string `json:"id"`
			Competencies []struct {
				Main struct {
					ID string `json:"id"`
				} `json:"main"`
				Basic []struct {
					ID string `json:"id"`
				} `json:"basic"`
			} `json:"competencies"`
		}
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &value) != nil || len(value.Competencies) != 1 || len(value.Competencies[0].Basic) != 2 {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		return value.ID, value.Competencies[0].Main.ID, value.Competencies[0].Basic[0].ID
	}
	variantID, mainID, basicID := create("assessment-variant")
	_, mismatchedTaskID, _ := create("assessment-other-variant")
	reader := variantpg.New(f.pool)
	before, err := reader.TaskForGrading(context.Background(), ownerID, variantID, mainID)
	if err != nil {
		t.Fatal(err)
	}
	replacement := strings.ReplaceAll(string(variantMapCSV(t)), "сумма признаков с весами", "НОВЫЙ ЭТАЛОН")
	if w := upload(f, "/admin/competency-map/import", "file", "replacement.csv", "text/csv", []byte(replacement), admin); w.Code != 200 {
		t.Fatalf("reimport: %d %s", w.Code, w.Body.String())
	}
	var mode atomic.Value
	mode.Store("correct")
	contexts := make(chan assessmentapp.GradingContext, 32)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Errorf("model request: %v", err)
			w.WriteHeader(500)
			return
		}
		var grading assessmentapp.GradingContext
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &grading); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		contexts <- grading
		currentMode := mode.Load().(string)
		if currentMode == "unavailable" {
			w.WriteHeader(503)
			return
		}
		if currentMode == "timeout" {
			w.WriteHeader(504)
			return
		}
		score, verdict := grading.MaxScore, "correct"
		items := []map[string]any{}
		for _, criterion := range grading.Criteria {
			item := map[string]any{"key": criterion.Key, "satisfied": true, "explanation": "Критерий выполнен."}
			if criterion.Mandatory && currentMode == "mandatory failed" {
				item["satisfied"] = false
				score, verdict = 0, "incorrect"
			}
			items = append(items, item)
		}
		switch currentMode {
		case "partial":
			items[0]["satisfied"] = false
			score, verdict = 1, "partial"
		case "missing satisfied":
			delete(items[0], "satisfied")
			score, verdict = 0, "incorrect"
		case "null satisfied":
			items[0]["satisfied"] = nil
			score, verdict = 0, "incorrect"
		case "wrong key":
			items[0]["key"] = "unexpected"
		case "basic two":
			score = 2
		}
		feedback := []string{"Итог.", "Причина.", "Совет."}
		if currentMode == "short feedback" {
			feedback = feedback[:2]
		}
		content, _ := json.Marshal(map[string]any{"score": score, "verdict": verdict, "criterion_results": items, "feedback": feedback})
		if currentMode == "malformed" {
			content = []byte("{")
		}
		finish := "stop"
		if currentMode == "truncated" {
			finish = "length"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}, "finish_reason": finish}}})
	}))
	t.Cleanup(provider.Close)
	f.app.cfg.AssessmentBaseURL = provider.URL
	evaluate := func(transcriptionID, taskID string, cookie *http.Cookie) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"transcription_id": transcriptionID, "variant_id": variantID, "variant_task_id": taskID})
		return f.request("POST", "/assessments/evaluate", string(body), cookie)
	}
	requireCode(t, evaluate("foreign-answer", mainID, access), 404, "TRANSCRIPTION_NOT_FOUND")
	if w := evaluate("foreign-answer", mainID, other); w.Code != 404 {
		t.Fatalf("foreign variant: %d %s", w.Code, w.Body.String())
	}
	if w := evaluate("owned-answer", mismatchedTaskID, access); w.Code != 404 {
		t.Fatalf("mismatched position: %d %s", w.Code, w.Body.String())
	}
	if len(contexts) != 0 {
		t.Fatal("unauthorized resources reached model")
	}
	for _, tc := range []struct {
		name, taskID string
		score, max   int
	}{{"main", mainID, 2, 2}, {"basic", basicID, 1, 1}, {"partial", mainID, 1, 2}, {"mandatory failed", mainID, 0, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "mandatory failed" || tc.name == "partial" {
				mode.Store(tc.name)
			} else {
				mode.Store("correct")
			}
			w := evaluate("owned-answer", tc.taskID, access)
			var result assessmenthttp.EvaluateResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Score != tc.score || result.MaxScore != tc.max || len(result.Feedback) != 3 {
				t.Fatalf("evaluation: %d %s", w.Code, w.Body.String())
			}
			grading := <-contexts
			if grading.MaxScore != tc.max || grading.Role != map[bool]string{true: "main", false: "basic"}[tc.taskID == mainID] {
				t.Fatalf("untrusted scale: %+v", grading)
			}
			if tc.taskID == mainID && grading.ReferenceAnswer != *before.Task.ReferenceAnswer {
				t.Fatalf("lost historical reference: %q", grading.ReferenceAnswer)
			}
		})
	}
	for _, name := range []string{"missing satisfied", "null satisfied", "wrong key", "basic two", "short feedback", "malformed", "truncated", "unavailable", "timeout"} {
		t.Run(name, func(t *testing.T) {
			mode.Store(name)
			taskID := mainID
			if name == "basic two" {
				taskID = basicID
			}
			status, code := 502, "INVALID_MODEL_RESPONSE"
			if name == "unavailable" {
				status, code = 503, "MODEL_UNAVAILABLE"
			}
			if name == "timeout" {
				status, code = 504, "MODEL_TIMEOUT"
			}
			requireCode(t, evaluate("owned-answer", taskID, access), status, code)
			<-contexts
		})
	}
}
