package app

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	diagnostichttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/transport/http"
)

func TestDiagnosticSessionAPIProgressOwnershipAndSnapshotPrivacy(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	f.handler = f.app.Handler()
	access, _, _ := f.register(t, "diagnostic@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	variantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(""))
	variantRequest.Header.Set("Idempotency-Key", "diagnostic-variant-1")
	variantRequest.AddCookie(access)
	variantResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(variantResponse, variantRequest)
	if variantResponse.Code != http.StatusCreated {
		t.Fatalf("create variant: %d %s", variantResponse.Code, variantResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variantResponse.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("invalid variant response: %s, %v", variantResponse.Body.String(), err)
	}
	startBody, _ := json.Marshal(diagnostichttp.StartRequest{VariantID: created.ID})
	startRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(startBody))
	startRequest.Header.Set("Content-Type", "application/json")
	startRequest.Header.Set("Idempotency-Key", "diagnostic-session-1")
	startRequest.AddCookie(access)
	startResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", startResponse.Code, startResponse.Body.String())
	}
	var progress diagnostichttp.ProgressResponse
	if err := json.Unmarshal(startResponse.Body.Bytes(), &progress); err != nil || progress.Current == nil || progress.Current.Role != "main" {
		t.Fatalf("invalid start response: %s, %v", startResponse.Body.String(), err)
	}
	if strings.Contains(startResponse.Body.String(), "Ответ A") || strings.Contains(startResponse.Body.String(), "ОС") || strings.Contains(startResponse.Body.String(), "reference_answer") {
		t.Fatal("session start exposed grading-only snapshot data")
	}
	malformedQuery := httptest.NewRequest(http.MethodGet, "https://api.example/diagnostic-sessions/"+progress.SessionID+"?%zz", nil)
	malformedQuery.AddCookie(access)
	malformedQueryResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(malformedQueryResponse, malformedQuery)
	if malformedQueryResponse.Code != 422 {
		t.Fatalf("malformed query was not rejected: %d %s", malformedQueryResponse.Code, malformedQueryResponse.Body.String())
	}
	replayRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(startBody))
	replayRequest.Header.Set("Content-Type", "application/json")
	replayRequest.Header.Set("Idempotency-Key", "diagnostic-session-1")
	replayRequest.AddCookie(access)
	replayResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusOK || !strings.Contains(replayResponse.Body.String(), progress.SessionID) {
		t.Fatalf("start idempotency replay: %d %s", replayResponse.Code, replayResponse.Body.String())
	}
	if unauthorized := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID, ""); unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized session read: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	other, _, _ := f.register(t, "diagnostic-other@example.edu")
	if foreign := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID, "", other); foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign session read: %d %s", foreign.Code, foreign.Body.String())
	}
	if before := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/result", "", access); before.Code != http.StatusConflict {
		t.Fatalf("active result: %d %s", before.Code, before.Body.String())
	}
	audioResponse := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio", "", access)
	if audioResponse.Code != http.StatusOK || audioResponse.Header().Get("Content-Type") != "audio/wav" {
		t.Fatalf("current instruction audio: %d %s", audioResponse.Code, audioResponse.Body.String())
	}
	answerBody, contentType := diagnosticAnswerBody(t, progress.Current.ID)
	answerRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions/"+progress.SessionID+"/answers", bytes.NewReader(answerBody.Bytes()))
	answerRequest.Header.Set("Content-Type", contentType)
	answerRequest.Header.Set("Idempotency-Key", "diagnostic-answer-1")
	answerRequest.AddCookie(access)
	answerResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(answerResponse, answerRequest)
	if answerResponse.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", answerResponse.Code, answerResponse.Body.String())
	}
	var completed diagnostichttp.ProgressResponse
	if err := json.Unmarshal(answerResponse.Body.Bytes(), &completed); err != nil || completed.Status != "completed" || completed.Score == nil || *completed.Score != 2 || completed.Skipped != 2 {
		t.Fatalf("invalid answer response: %s, %v", answerResponse.Body.String(), err)
	}
	result := f.request(http.MethodGet, "/diagnostic-sessions/"+completed.SessionID+"/result", "", access)
	if result.Code != http.StatusOK || strings.Contains(result.Body.String(), "reference_answer") || strings.Contains(result.Body.String(), `"criteria":"criterion"`) {
		t.Fatalf("result or private-field exposure: %d %s", result.Code, result.Body.String())
	}
	var resultBody struct {
		DiagnosticScore int `json:"diagnostic_score"`
		MaximumScore    int `json:"maximum_score"`
		Answers         []struct {
			GraderScore int `json:"grader_score"`
			Score       int `json:"score"`
		} `json:"answers"`
		UntestedBasics []map[string]any `json:"untested_basics"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &resultBody); err != nil || resultBody.DiagnosticScore != 2 || resultBody.MaximumScore != 2 || len(resultBody.Answers) != 1 || len(resultBody.UntestedBasics) != 2 {
		t.Fatalf("invalid result data: %s, %v", result.Body.String(), err)
	}
}

func diagnosticAnswerBody(t *testing.T, taskID string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("variant_task_id", taskID); err != nil {
		t.Fatal(err)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="audio"; filename="answer.wav"`)
	header.Set("Content-Type", "audio/wav")
	file, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(wavBytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
