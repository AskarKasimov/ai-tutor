package app

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	diagnostichttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/transport/http"
)

func TestDiagnosticSkipAdvancesWithoutAudio(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	f.handler = f.app.Handler()
	access, _, _ := f.register(t, "diagnostic-skip@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	post := func(path, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(access)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	variant := post("/variants", "skip-variant", `{"subject_id":"subject:test"}`)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variant.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("variant: %d %s", variant.Code, variant.Body.String())
	}
	started := post("/diagnostic-sessions", "skip-start", `{"variant_id":"`+created.ID+`"}`)
	var progress diagnostichttp.ProgressResponse
	if err := json.Unmarshal(started.Body.Bytes(), &progress); err != nil || progress.Current == nil {
		t.Fatalf("start: %d %s", started.Code, started.Body.String())
	}
	request := `{"variant_task_id":"` + progress.Current.ID + `"}`
	skipped := post("/diagnostic-sessions/"+progress.SessionID+"/skip", "skip-task", request)
	if err := json.Unmarshal(skipped.Body.Bytes(), &progress); err != nil || skipped.Code != 200 || !progress.AnswerSkipped || progress.Score == nil || *progress.Score != 0 || progress.Current == nil {
		t.Fatalf("skip: %d %s", skipped.Code, skipped.Body.String())
	}
	replay := post("/diagnostic-sessions/"+progress.SessionID+"/skip", "skip-task", request)
	if replay.Code != 200 || replay.Body.String() != skipped.Body.String() {
		t.Fatalf("skip replay: %d %s", replay.Code, replay.Body.String())
	}
}

func TestDiagnosticSessionAPIProgressOwnershipAndSnapshotPrivacy(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	f.handler = f.app.Handler()
	access, _, _ := f.register(t, "diagnostic@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	variantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:test"}`))
	variantRequest.Header.Set("Content-Type", "application/json")
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
	foreignAudio := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio?variant_task_id="+progress.Current.ID, "", other)
	requireCode(t, foreignAudio, http.StatusNotFound, "DIAGNOSTIC_SESSION_NOT_FOUND")
	if before := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/result", "", access); before.Code != http.StatusConflict {
		t.Fatalf("active result: %d %s", before.Code, before.Body.String())
	}
	audioResponse := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio?variant_task_id="+progress.Current.ID, "", access)
	var audioMetadata struct {
		VariantTaskID string  `json:"variant_task_id"`
		Status        string  `json:"status"`
		AudioURL      *string `json:"audio_url"`
	}
	if err := json.Unmarshal(audioResponse.Body.Bytes(), &audioMetadata); audioResponse.Code != http.StatusOK || audioResponse.Header().Get("Content-Type") != "application/json; charset=utf-8" || err != nil || audioMetadata.VariantTaskID != progress.Current.ID || audioMetadata.Status != "pending" || audioMetadata.AudioURL != nil {
		t.Fatalf("current instruction audio: %d %s", audioResponse.Code, audioResponse.Body.String())
	}
	staleAudio := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio?variant_task_id=stale-task", "", access)
	requireCode(t, staleAudio, http.StatusConflict, "DIAGNOSTIC_TASK_CHANGED")
	var audioID string
	if err := f.pool.QueryRow(t.Context(), "SELECT audio_asset_id FROM variant_tasks WHERE id=$1", progress.Current.ID).Scan(&audioID); err != nil || audioID == "" {
		t.Fatalf("variant task audio link: id=%q err=%v", audioID, err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE audio_assets SET status='ready', bucket='persisted-bucket', storage_uri='s3://persisted-bucket/task-audio/v1/'||id||'.wav', audio_url='/task-audio/'||id||'/file' WHERE id=$1", audioID); err != nil {
		t.Fatal(err)
	}
	f.s3Mu.Lock()
	f.s3AssetID = audioID
	f.s3Mu.Unlock()
	readyMetadata := f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio?variant_task_id="+progress.Current.ID, "", access)
	if readyMetadata.Code != http.StatusOK || !strings.Contains(readyMetadata.Body.String(), `"status":"ready"`) || !strings.Contains(readyMetadata.Body.String(), `"audio_url":"/task-audio/`+audioID+`/file"`) {
		t.Fatalf("ready metadata: %d %s", readyMetadata.Code, readyMetadata.Body.String())
	}
	for range 10 {
		fileResponse := f.request(http.MethodGet, "/task-audio/"+audioID+"/file", "", access)
		if fileResponse.Code != http.StatusOK || fileResponse.Header().Get("Content-Type") != "audio/wav" || fileResponse.Header().Get("Cache-Control") != "private, no-store" || !bytes.Equal(fileResponse.Body.Bytes(), wavBytes()) {
			t.Fatalf("saved audio file: %d headers=%v body=%q", fileResponse.Code, fileResponse.Header(), fileResponse.Body.String())
		}
	}
	f.s3Mu.Lock()
	paths := append([]string(nil), f.s3Paths...)
	f.s3Mu.Unlock()
	if len(paths) != 10 {
		t.Fatalf("S3 requests=%d, wanted ten saved-file reads", len(paths))
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "/persisted-bucket/task-audio/v1/") {
			t.Fatalf("S3 requested path %q; wanted persisted bucket/key", path)
		}
	}
	if foreignFile := f.request(http.MethodGet, "/task-audio/"+audioID+"/file", "", other); foreignFile.Code != http.StatusOK {
		t.Fatalf("published task audio access: %d %s", foreignFile.Code, foreignFile.Body.String())
	}
	archivedAssetID := "snapshot-only-audio"
	if _, err := f.pool.Exec(t.Context(), "INSERT INTO audio_assets(id,instruction,object_key) VALUES ($1,'Archived instruction',$2)", archivedAssetID, "task-audio/v1/"+archivedAssetID+".wav"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE variant_tasks SET audio_asset_id=$2 WHERE id=$1", progress.Current.ID, archivedAssetID); err != nil {
		t.Fatal(err)
	}
	if ownPending := f.request(http.MethodGet, "/task-audio/"+archivedAssetID+"/file", "", access); ownPending.Code != http.StatusConflict {
		t.Fatalf("owner could not reach archived snapshot asset: %d %s", ownPending.Code, ownPending.Body.String())
	}
	if foreignArchived := f.request(http.MethodGet, "/task-audio/"+archivedAssetID+"/file", "", other); foreignArchived.Code != http.StatusNotFound {
		t.Fatalf("foreign archived audio: %d %s", foreignArchived.Code, foreignArchived.Body.String())
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
	if err := json.Unmarshal(answerResponse.Body.Bytes(), &completed); err != nil || completed.Status != "completed" || completed.Score == nil || *completed.Score != 2 || completed.GraderMaxScore == nil || *completed.GraderMaxScore != 2 || completed.Skipped != 2 {
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
			GraderScore    int `json:"grader_score"`
			GraderMaxScore int `json:"grader_max_score"`
			Score          int `json:"score"`
		} `json:"answers"`
		UntestedBasics []map[string]any `json:"untested_basics"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &resultBody); err != nil || resultBody.DiagnosticScore != 2 || resultBody.MaximumScore != 2 || len(resultBody.Answers) != 1 || resultBody.Answers[0].GraderMaxScore != 2 || len(resultBody.UntestedBasics) != 2 {
		t.Fatalf("invalid result data: %s, %v", result.Body.String(), err)
	}
	feedbackResp := f.request(http.MethodGet, "/diagnostic-sessions/"+completed.SessionID+"/feedback", "", access)
	if feedbackResp.Code != http.StatusOK {
		t.Fatalf("feedback: %d %s", feedbackResp.Code, feedbackResp.Body.String())
	}
	var fbBody struct {
		SessionID       string   `json:"session_id"`
		DiagnosticScore int      `json:"diagnostic_score"`
		MaximumScore    int      `json:"maximum_score"`
		Summary         string   `json:"summary"`
		Strengths       []string `json:"strengths"`
	}
	if err := json.Unmarshal(feedbackResp.Body.Bytes(), &fbBody); err != nil || fbBody.SessionID != completed.SessionID || fbBody.Summary == "" || len(fbBody.Strengths) != 1 {
		t.Fatalf("invalid feedback data: %s, %v", feedbackResp.Body.String(), err)
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
	// Exercise the multipart upload limit, rather than only the 32 KiB JSON limit.
	audio := append(wavBytes(), make([]byte, 64*1024)...)
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	binary.LittleEndian.PutUint32(audio[40:44], uint32(len(audio)-44))
	if _, err := file.Write(audio); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
