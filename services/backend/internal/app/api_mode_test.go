package app

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	assessmenthttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/transport/http"
	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
)

func TestMockModeUsesLocalModelsWithRealAuthAndStorage(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(provider.Close)
	t.Cleanup(func() {
		if calls.Load() != 0 {
			t.Errorf("mock mode contacted models %d times", calls.Load())
		}
	})
	f.app.cfg.STTURL = provider.URL + "/transcribe"
	f.app.cfg.TTSURL = provider.URL + "/synthesize"
	f.app.cfg.AssessmentBaseURL = provider.URL + "/v1"

	requireCode(t, f.request("POST", "/voice/syntheses", `{"text":"Вопрос"}`), 401, "UNAUTHORIZED")
	access, _, ownerID := f.register(t, "mock@example.edu")
	requireCode(t, f.request("POST", "/voice/syntheses", `{"text":""}`, access), 422, "VALIDATION_ERROR")
	requireCode(t, upload(f, "/voice/transcriptions", "audio", "bad.wav", "audio/wav", []byte("bad"), access), 422, "INVALID_AUDIO")

	w := upload(f, "/voice/transcriptions", "audio", "answer.wav", "audio/wav", wavBytes(), access)
	if w.Code != http.StatusOK {
		t.Fatalf("mock STT: %d %s", w.Code, w.Body.String())
	}
	var tr voicehttp.Transcription
	if err := json.Unmarshal(w.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.ID == "" || !strings.Contains(tr.Text, "Демонстрацион") || tr.CreatedAt != f.now.Unix() {
		t.Fatalf("invalid demo transcription: %+v", tr)
	}

	var storedOwner, storedText string
	if err := f.pool.QueryRow(context.Background(), "SELECT user_id,text FROM transcriptions WHERE id=$1", tr.ID).Scan(&storedOwner, &storedText); err != nil {
		t.Fatal(err)
	}
	if storedOwner != ownerID || storedText != tr.Text {
		t.Fatal("mock transcription was not persisted for the real user")
	}

	w = f.request("POST", "/voice/syntheses", `{"text":"Озвучьте вопрос"}`, access)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "audio/wav" || !audio.ValidWAV(w.Body.Bytes()) {
		t.Fatalf("mock TTS must return playable WAV: %d", w.Code)
	}
	var audible bool
	for pos := 44; pos+2 <= w.Body.Len(); pos += 2 {
		if binary.LittleEndian.Uint16(w.Body.Bytes()[pos:pos+2]) != 0 {
			audible = true
			break
		}
	}
	if !audible {
		t.Fatal("demo WAV contains no audible cue")
	}

	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import competency map: %d %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(""))
	req.Header.Set("Idempotency-Key", "mock-assessment-variant")
	req.AddCookie(access)
	w = httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create variant: %d %s", w.Code, w.Body.String())
	}

	var created struct {
		ID           string `json:"id"`
		Competencies []struct {
			Main struct {
				ID string `json:"id"`
			} `json:"main"`
		} `json:"competencies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Competencies) == 0 || created.Competencies[0].Main.ID == "" {
		t.Fatalf("invalid created variant: %s", w.Body.String())
	}

	body, _ := json.Marshal(map[string]string{
		"transcription_id": tr.ID,
		"variant_id":       created.ID,
		"variant_task_id":  created.Competencies[0].Main.ID,
	})
	w = f.request("POST", "/assessments/evaluate", string(body), access)
	if w.Code != http.StatusOK {
		t.Fatalf("mock assessment: %d %s", w.Code, w.Body.String())
	}

	var result assessmenthttp.EvaluateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Score != 2 || result.MaxScore != 2 || result.Verdict != "correct" || len(result.CriterionResults) == 0 || len(result.Feedback) != 3 || !strings.Contains(strings.Join(result.Feedback, " "), "Демонстрацион") {
		t.Fatalf("invalid demo assessment: %+v", result)
	}

	otherAccess, _, _ := f.register(t, "other-mock@example.edu")
	w = f.request("POST", "/assessments/evaluate", string(body), otherAccess)
	if w.Code != http.StatusNotFound {
		t.Fatalf("mock mode leaked another user's variant/transcription: %d %s", w.Code, w.Body.String())
	}
}
