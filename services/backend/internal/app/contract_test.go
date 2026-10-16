package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/csvparser"
	trainingpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/postgres"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// Compile the complete contract, including planned schemas, without a database.
func TestOpenAPIContractSchemas(t *testing.T) {
	data, err := os.ReadFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	const resource = "https://tutor.example/openapi.json"
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(resource, doc); err != nil {
		t.Fatal(err)
	}
	for name := range spec["components"].(map[string]any)["schemas"].(map[string]any) {
		if _, err := compiler.Compile(resource + "#/components/schemas/" + name); err != nil {
			t.Errorf("invalid schema %s: %v", name, err)
		}
	}
	for path, node := range spec["paths"].(map[string]any) {
		for method, value := range node.(map[string]any) {
			if !strings.Contains(" get post put patch delete head options ", " "+method+" ") {
				continue
			}
			status := value.(map[string]any)["x-implementation"]
			if status != "implemented" && status != "planned" {
				t.Errorf("%s %s requires explicit implementation status", method, path)
			}
		}
	}
}

// Validate actual handler responses using only implemented contract operations.
func TestOpenAPIResponses(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	data, err := os.ReadFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const resource = "https://tutor.example/openapi.json"
	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	if err = compiler.AddResource(resource, doc); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	checked := map[string]bool{}
	check := func(method, path string, w *httptest.ResponseRecorder) {
		t.Helper()
		op := paths[path].(map[string]any)[strings.ToLower(method)].(map[string]any)
		response, ok := op["responses"].(map[string]any)[fmt.Sprint(w.Code)]
		if !ok {
			t.Fatalf("response status not in contract: %s %s %d %s", method, path, w.Code, w.Body.String())
		}
		node := response.(map[string]any)
		if ref, ok := node["$ref"].(string); ok {
			node = spec["components"].(map[string]any)["responses"].(map[string]any)[strings.TrimPrefix(ref, "#/components/responses/")].(map[string]any)
		}
		if w.Code == 204 {
			if w.Body.Len() != 0 {
				t.Fatal("204 response has body")
			}
			checked[path] = true
			return
		}
		content := node["content"].(map[string]any)
		if _, ok := content["audio/wav"]; ok {
			if w.Header().Get("Content-Type") != "audio/wav" || !audio.ValidWAV(w.Body.Bytes()) {
				t.Fatal("invalid WAV response")
			}
			checked[path] = true
			return
		}
		if _, ok := content["application/octet-stream"]; ok {
			if w.Body.Len() == 0 {
				t.Fatal("empty octet-stream response")
			}
			checked[path] = true
			return
		}
		schemaNode := content["application/json"].(map[string]any)["schema"].(map[string]any)
		ref, ok := schemaNode["$ref"].(string)
		if !ok {
			escape := strings.NewReplacer("~", "~0", "/", "~1")
			ref = "#/paths/" + escape.Replace(path) + "/" + strings.ToLower(method) +
				"/responses/" + fmt.Sprint(w.Code) + "/content/application~1json/schema"
		}
		schema, err := compiler.Compile(resource + ref)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err = json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if err = schema.Validate(value); err != nil {
			t.Fatalf("contract mismatch %s %d: %v", path, w.Code, err)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("wrong response type")
		}
		if w.Code < 300 {
			checked[path] = true
		}
	}
	register := f.request("POST", "/auth/register", `{"email":"contract@example.edu","password":"`+password+`","display_name":"Контракт"}`)
	check("POST", "/auth/register", register)
	cookies := register.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatal("missing cookies")
	}
	access, refresh := cookies[0], cookies[1]
	check("GET", "/auth/me", f.request("GET", "/auth/me", "", access))
	check("GET", "/auth/me", f.request("GET", "/auth/me", ""))
	check("POST", "/auth/login", f.request("POST", "/auth/login", `{"email":"contract@example.edu","password":"`+password+`"}`))
	check("POST", "/auth/teacher", f.portal("POST", "/auth/teacher", ""))
	check("POST", "/auth/teacher", f.request("POST", "/auth/teacher", ""))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/synthesize":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wavBytes())
		case "/transcribe":
			httpx.JSON(w, 200, map[string]any{"text": "Ответ.", "model": "v3", "segments": []any{}})
		case "/chat/completions":
			serveFixtureCompletion(t, w, r)
		default:
			t.Errorf("unexpected provider path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	f.app.cfg.STTURL = provider.URL + "/transcribe"
	f.app.cfg.TTSURL = provider.URL + "/synthesize"
	f.app.cfg.AssessmentBaseURL = provider.URL
	f.app.cfg.TaskgenBaseURL = provider.URL
	transcription := upload(f, "/voice/transcriptions", "audio", "answer.wav", "audio/wav", wavBytes(), access)
	check("POST", "/voice/transcriptions", transcription)
	var saved struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(transcription.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if transcription.Code != http.StatusOK || saved.ID == "" {
		t.Fatalf("transcription: %d %s", transcription.Code, transcription.Body.String())
	}
	check("POST", "/voice/syntheses", f.request("POST", "/voice/syntheses", `{"text":"Вопрос?"}`, access))
	check("POST", "/voice/syntheses", f.request("POST", "/voice/syntheses", `{"text":""}`, access))
	admin := f.admin(t)
	check("POST", "/admin/subjects", f.request("POST", "/admin/subjects", `{"name":"Предмет контракта"}`, admin))
	check("GET", "/subjects", f.request("GET", "/subjects", "", admin))
	materialsUpload := upload(f, "/admin/subjects/subject:test/materials", "file", "lecture.md", "text/markdown", []byte("# Введение\nФормула $x=1$"), admin)
	check("POST", "/admin/subjects/{subject_id}/materials", materialsUpload)
	var uploadResult struct {
		Uploaded []struct {
			ID string `json:"id"`
		} `json:"uploaded"`
		Failed []struct {
			Filename string `json:"filename"`
		} `json:"failed"`
	}
	if err := json.Unmarshal(materialsUpload.Body.Bytes(), &uploadResult); err != nil || len(uploadResult.Uploaded) == 0 {
		t.Fatalf("materials upload: %s (%v)", materialsUpload.Body.String(), err)
	}
	matID := uploadResult.Uploaded[0].ID
	check("GET", "/admin/subjects/{subject_id}/materials", f.request("GET", "/admin/subjects/subject:test/materials", "", admin))
	check("GET", "/admin/subjects/{subject_id}/graph", f.request("GET", "/admin/subjects/subject:test/graph", "", admin))
	check("GET", "/admin/subjects/{subject_id}/graph", f.request("GET", "/admin/subjects/subject:test/graph", "", access))
	check("GET", "/admin/subjects/{subject_id}/materials/{material_id}/raw", f.request("GET", "/admin/subjects/subject:test/materials/"+matID+"/raw", "", admin))
	check("DELETE", "/admin/subjects/{subject_id}/materials/{material_id}", f.request("DELETE", "/admin/subjects/subject:test/materials/"+matID, "", admin))
	check("POST", "/admin/subjects/{subject_id}/competency-map/import", upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", []byte(mapCSV), admin))
	if variantImport := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "variant.csv", "text/csv", variantMapCSV(t), admin); variantImport.Code != http.StatusOK {
		t.Fatalf("variant map import: %d %s", variantImport.Code, variantImport.Body.String())
	}
	check("POST", "/admin/subjects/{subject_id}/competency-map/import", upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "subject.csv", "text/csv", variantMapCSV(t), admin))
	check("GET", "/subjects/{subject_id}/competency-map", f.request("GET", "/subjects/subject:test/competency-map", "", access))
	variantCreateRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:test"}`))
	variantCreateRequest.Header.Set("Content-Type", "application/json")
	variantCreateRequest.Header.Set("Idempotency-Key", "contract-variant-1")
	variantCreateRequest.AddCookie(access)
	variantCreate := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(variantCreate, variantCreateRequest)
	check("POST", "/variants", variantCreate)
	var createdVariant struct {
		ID           string `json:"id"`
		Competencies []struct {
			Main struct {
				ID string `json:"id"`
			} `json:"main"`
		} `json:"competencies"`
	}
	if err := json.Unmarshal(variantCreate.Body.Bytes(), &createdVariant); err != nil || len(createdVariant.Competencies) != 1 {
		t.Fatalf("contract variant response: %s (%v)", variantCreate.Body.String(), err)
	}
	var contractAudioID string
	if err := f.pool.QueryRow(t.Context(), "SELECT audio_asset_id FROM variant_tasks WHERE id=$1", createdVariant.Competencies[0].Main.ID).Scan(&contractAudioID); err != nil || contractAudioID == "" {
		t.Fatalf("contract task audio link: %q (%v)", contractAudioID, err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE audio_assets SET status='ready', bucket='contract-bucket', storage_uri='s3://contract-bucket/'||object_key, audio_url='/task-audio/'||id||'/file' WHERE id=$1", contractAudioID); err != nil {
		t.Fatal(err)
	}
	f.s3Mu.Lock()
	f.s3AssetID = contractAudioID
	f.s3Mu.Unlock()
	check("GET", "/task-audio/{id}/file", f.request("GET", "/task-audio/"+contractAudioID+"/file", "", access))
	evaluateBody, err := json.Marshal(map[string]string{
		"transcription_id": saved.ID,
		"variant_id":       createdVariant.ID,
		"variant_task_id":  createdVariant.Competencies[0].Main.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := f.request("POST", "/assessments/evaluate", string(evaluateBody), access)
	if evaluation.Code != http.StatusOK {
		t.Fatalf("assessment: %d %s", evaluation.Code, evaluation.Body.String())
	}
	check("POST", "/assessments/evaluate", evaluation)
	diagnosticStartBody, _ := json.Marshal(map[string]string{"variant_id": createdVariant.ID})
	diagnosticStartRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(diagnosticStartBody))
	diagnosticStartRequest.Header.Set("Content-Type", "application/json")
	diagnosticStartRequest.Header.Set("Idempotency-Key", "contract-diagnostic-1")
	diagnosticStartRequest.AddCookie(access)
	diagnosticStart := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(diagnosticStart, diagnosticStartRequest)
	check("POST", "/diagnostic-sessions", diagnosticStart)
	var diagnosticProgress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(diagnosticStart.Body.Bytes(), &diagnosticProgress); err != nil || diagnosticProgress.SessionID == "" || diagnosticProgress.Current.ID == "" {
		t.Fatalf("diagnostic start response: %s (%v)", diagnosticStart.Body.String(), err)
	}
	check("GET", "/diagnostic-sessions/{id}", f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID, "", access))
	check("GET", "/diagnostic-sessions/{id}/current/audio", f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/current/audio?variant_task_id="+diagnosticProgress.Current.ID, "", access))
	var diagnosticAudioID, diagnosticAudioKey string
	if err := f.pool.QueryRow(t.Context(), "SELECT audio_asset_id FROM variant_tasks WHERE id=$1", diagnosticProgress.Current.ID).Scan(&diagnosticAudioID); err != nil || diagnosticAudioID == "" {
		t.Fatalf("diagnostic task audio link: %q (%v)", diagnosticAudioID, err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE audio_assets SET status='ready', bucket='task-audio-test', storage_uri='s3://task-audio-test/'||object_key, audio_url='/task-audio/'||id||'/file' WHERE id=$1", diagnosticAudioID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), "SELECT object_key FROM audio_assets WHERE id=$1", diagnosticAudioID).Scan(&diagnosticAudioKey); err != nil {
		t.Fatal(err)
	}
	f.s3Mu.Lock()
	f.s3AssetID = diagnosticAudioID
	f.s3Objects[diagnosticAudioKey] = fixtureS3Object{data: wavBytes(), assetID: diagnosticAudioID}
	f.s3Mu.Unlock()
	regenerateBody, _ := json.Marshal(map[string]string{"variant_task_id": diagnosticProgress.Current.ID})
	regenerate := f.request("POST", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/current/audio/regenerate", string(regenerateBody), access)
	if regenerate.Code != http.StatusOK {
		t.Fatalf("regenerate contract response: %d %s", regenerate.Code, regenerate.Body.String())
	}
	check("POST", "/diagnostic-sessions/{id}/current/audio/regenerate", regenerate)
	check("GET", "/task-audio/{id}/file", f.request("GET", "/task-audio/unknown/file", "", access))
	diagnosticAnswer, diagnosticContentType := diagnosticAnswerBody(t, diagnosticProgress.Current.ID)
	diagnosticAnswerRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions/"+diagnosticProgress.SessionID+"/answers", diagnosticAnswer)
	diagnosticAnswerRequest.Header.Set("Content-Type", diagnosticContentType)
	diagnosticAnswerRequest.Header.Set("Idempotency-Key", "contract-diagnostic-answer-1")
	diagnosticAnswerRequest.AddCookie(access)
	diagnosticAnswerResponse := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(diagnosticAnswerResponse, diagnosticAnswerRequest)
	check("POST", "/diagnostic-sessions/{id}/answers", diagnosticAnswerResponse)
	check("GET", "/diagnostic-sessions/{id}/result", f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/result", "", access))
	check("GET", "/diagnostic-sessions/{id}/feedback", f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/feedback", "", access))
	check("GET", "/subjects/{subject_id}/learning-state", f.request("GET", "/subjects/subject:test/learning-state", "", access))
	preview := f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/training/preview", "", access)
	check("GET", "/diagnostic-sessions/{id}/training/preview", preview)
	var trainingPreview training.Preview
	if err = json.Unmarshal(preview.Body.Bytes(), &trainingPreview); err != nil {
		t.Fatal(err)
	}
	f.handler = f.app.Handler()
	trainingStart := trainingJSON(f, access, "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/training", "contract-training", fmt.Sprintf(`{"plan_revision":%d}`, trainingPreview.PlanRevision))
	check("POST", "/diagnostic-sessions/{id}/training", trainingStart)
	var trainingProgress training.Progress
	if err = json.Unmarshal(trainingStart.Body.Bytes(), &trainingProgress); err != nil {
		t.Fatal(err)
	}
	check("GET", "/diagnostic-sessions/{id}/training", f.request("GET", "/diagnostic-sessions/"+diagnosticProgress.SessionID+"/training", "", access))
	check("GET", "/training-sessions/{id}", f.request("GET", "/training-sessions/"+trainingProgress.ID, "", access))
	check("GET", "/training-sessions/{id}/current/audio", f.request("GET", "/training-sessions/"+trainingProgress.ID+"/current/audio?exercise_id="+trainingProgress.Current.ID, "", access))
	trainingBody, trainingContentType := diagnosticAnswerBody(t, trainingProgress.Current.ID)
	trainingRequest := httptest.NewRequest("POST", "/training-sessions/"+trainingProgress.ID+"/answers", strings.NewReader(strings.ReplaceAll(trainingBody.String(), "variant_task_id", "exercise_id")))
	trainingRequest.AddCookie(access)
	trainingRequest.Header.Set("Content-Type", trainingContentType)
	trainingRequest.Header.Set("Idempotency-Key", "contract-training-answer")
	trainingAnswer := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(trainingAnswer, trainingRequest)
	check("POST", "/training-sessions/{id}/answers", trainingAnswer)
	check("GET", "/training-sessions/{id}/history", f.request("GET", "/training-sessions/"+trainingProgress.ID+"/history?limit=1", "", access))
	postJSON := func(path, key string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "https://api.example"+path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		request.AddCookie(access)
		response := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(response, request)
		return response
	}
	var answeredTraining training.Progress
	if err := json.Unmarshal(trainingAnswer.Body.Bytes(), &answeredTraining); err != nil || answeredTraining.Current.ID == "" {
		t.Fatalf("training answer response: %s (%v)", trainingAnswer.Body.String(), err)
	}
	trainingSkipBody, _ := json.Marshal(map[string]string{"exercise_id": answeredTraining.Current.ID})
	trainingSkip := postJSON("/training-sessions/"+trainingProgress.ID+"/skip", "contract-training-skip", trainingSkipBody)
	if trainingSkip.Code != http.StatusOK {
		t.Fatalf("training skip: %d %s", trainingSkip.Code, trainingSkip.Body.String())
	}
	check("POST", "/training-sessions/{id}/skip", trainingSkip)
	var skippedTraining training.Progress
	if err := json.Unmarshal(trainingSkip.Body.Bytes(), &skippedTraining); err != nil || skippedTraining.Current.ID == "" {
		t.Fatalf("training skip response: %s (%v)", trainingSkip.Body.String(), err)
	}
	chatPath := "/training-sessions/" + skippedTraining.ID + "/exercises/" + skippedTraining.Current.ID + "/chat"
	check("GET", "/training-sessions/{id}/exercises/{exercise_id}/chat", f.request("GET", chatPath, "", access))
	chatMessage := postJSON(chatPath+"/turns", "contract-chat-message", []byte(`{"revision":0,"kind":"message","text":"Как начать рассуждение?"}`))
	if chatMessage.Code != http.StatusOK {
		t.Fatalf("chat message: %d %s", chatMessage.Code, chatMessage.Body.String())
	}
	check("POST", "/training-sessions/{id}/exercises/{exercise_id}/chat/turns", chatMessage)
	chatTurn := postJSON(chatPath+"/turns", "contract-chat-check", []byte(`{"revision":1,"kind":"check","text":"Мой ответ на задание"}`))
	if chatTurn.Code != http.StatusOK {
		t.Fatalf("chat check: %d %s", chatTurn.Code, chatTurn.Body.String())
	}
	check("POST", "/training-sessions/{id}/exercises/{exercise_id}/chat/turns", chatTurn)
	var checkedTurn training.ChatTurnResult
	if err := json.Unmarshal(chatTurn.Body.Bytes(), &checkedTurn); err != nil || checkedTurn.TurnID == "" || checkedTurn.LatestCheck == nil {
		t.Fatalf("chat check response: %s (%v)", chatTurn.Body.String(), err)
	}
	check("POST", "/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/retry",
		postJSON(chatPath+"/turns/"+checkedTurn.TurnID+"/retry", "", []byte(`{}`)))
	var trainingOwner string
	if err := f.pool.QueryRow(t.Context(), "SELECT owner_id FROM training_sessions WHERE id=$1", skippedTraining.ID).Scan(&trainingOwner); err != nil {
		t.Fatal(err)
	}
	pending, _, err := trainingpg.NewChatRepository(f.pool).ReserveTurn(t.Context(), trainingOwner, skippedTraining.ID, training.TurnInput{
		ExerciseID: skippedTraining.Current.ID, Key: "contract-chat-cancel", Revision: checkedTurn.Revision,
		Kind: training.ChatKindMessage, Source: training.AnswerSourceVoice, Audio: wavBytes(), MediaType: "audio/wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	check("GET", "/training-sessions/{id}/exercises/{exercise_id}/chat", f.request("GET", chatPath, "", access))
	check("POST", "/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/cancel",
		postJSON(chatPath+"/turns/"+pending.Turn.ID+"/cancel", "", []byte(`{}`)))
	nextBody, _ := json.Marshal(map[string]int64{"revision": checkedTurn.Revision})
	next := postJSON("/training-sessions/"+skippedTraining.ID+"/exercises/"+skippedTraining.Current.ID+"/next", "contract-training-next", nextBody)
	if next.Code != http.StatusOK {
		t.Fatalf("training next: %d %s", next.Code, next.Body.String())
	}
	check("POST", "/training-sessions/{id}/exercises/{exercise_id}/next", next)
	// Confirming a preview is a separate operation and must not run STT again.
	confirmedStart := postJSON("/diagnostic-sessions", "contract-confirmed-start", diagnosticStartBody)
	var confirmedProgress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(confirmedStart.Body.Bytes(), &confirmedProgress); err != nil || confirmedProgress.Current.ID == "" {
		t.Fatal(confirmedStart.Body.String())
	}
	confirmedBody, _ := json.Marshal(map[string]string{"variant_task_id": confirmedProgress.Current.ID, "transcription_id": saved.ID})
	check("POST", "/diagnostic-sessions/{id}/answers/transcription", postJSON("/diagnostic-sessions/"+confirmedProgress.SessionID+"/answers/transcription", "contract-confirmed-answer", confirmedBody))

	diagnosticSkipStart := postJSON("/diagnostic-sessions", "contract-diagnostic-skip-start", diagnosticStartBody)
	if diagnosticSkipStart.Code != http.StatusCreated {
		t.Fatalf("diagnostic skip session: %d %s", diagnosticSkipStart.Code, diagnosticSkipStart.Body.String())
	}
	var diagnosticSkipProgress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(diagnosticSkipStart.Body.Bytes(), &diagnosticSkipProgress); err != nil || diagnosticSkipProgress.SessionID == "" || diagnosticSkipProgress.Current.ID == "" {
		t.Fatalf("diagnostic skip session response: %s (%v)", diagnosticSkipStart.Body.String(), err)
	}
	diagnosticSkipBody, _ := json.Marshal(map[string]string{"variant_task_id": diagnosticSkipProgress.Current.ID})
	diagnosticSkip := postJSON("/diagnostic-sessions/"+diagnosticSkipProgress.SessionID+"/skip", "contract-diagnostic-skip", diagnosticSkipBody)
	if diagnosticSkip.Code != http.StatusOK {
		t.Fatalf("diagnostic skip: %d %s", diagnosticSkip.Code, diagnosticSkip.Body.String())
	}
	check("POST", "/diagnostic-sessions/{id}/skip", diagnosticSkip)
	check("GET", "/variants", f.request("GET", "/variants?subject_id=subject%3Atest", "", access))
	check("GET", "/variants/{id}", f.request("GET", "/variants/"+createdVariant.ID, "", access))
	check("GET", "/variants/{id}/tasks/{task_id}", f.request("GET", "/variants/"+createdVariant.ID+"/tasks/"+createdVariant.Competencies[0].Main.ID, "", access))
	var outcomeID, taskID string
	if err := f.pool.QueryRow(context.Background(), `SELECT outcome.id, task.id FROM outcomes outcome JOIN tasks task ON task.outcome_id=outcome.id ORDER BY task.id LIMIT 1`).Scan(&outcomeID, &taskID); err != nil {
		t.Fatal(err)
	}
	check("GET", "/tasks", f.request("GET", "/tasks?subject_id=subject%3Atest", "", access))
	check("GET", "/tasks", f.request("GET", "/tasks?subject_id=subject%3Atest&importance=6", "", access))
	check("GET", "/tasks/{id}", f.request("GET", "/tasks/"+taskID+"?subject_id=subject%3Atest", "", access))
	requestBody, _ := json.Marshal(map[string]string{"subject_id": "subject:test", "outcome_id": outcomeID})
	check("POST", "/tasks/generate", f.request("POST", "/tasks/generate", string(requestBody), access))
	check("POST", "/tasks/generate", f.request("POST", "/tasks/generate", strings.Repeat(" ", 40*1024), access))
	generateRequest := httptest.NewRequest("POST", "https://api.example/tasks/generate", bytes.NewReader(requestBody))
	generateRequest.Header.Set("Content-Type", "application/json")
	generateRequest.Header.Set("Idempotency-Key", "contract-generation-1")
	generateRequest.AddCookie(access)
	generate := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(generate, generateRequest)
	check("POST", "/tasks/generate", generate)
	unavailable := httptest.NewServer(http.NotFoundHandler())
	unavailable.Close()
	f.app.cfg.TaskgenBaseURL = unavailable.URL
	unavailableRequest := httptest.NewRequest("POST", "/tasks/generate", bytes.NewReader(requestBody))
	unavailableRequest.Header.Set("Content-Type", "application/json")
	unavailableRequest.Header.Set("Idempotency-Key", "contract-generation-unavailable")
	unavailableRequest.AddCookie(access)
	unavailableResponse := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(unavailableResponse, unavailableRequest)
	if unavailableResponse.Code != 503 {
		t.Fatalf("unavailable generator: %d %s", unavailableResponse.Code, unavailableResponse.Body.String())
	}
	check("POST", "/tasks/generate", unavailableResponse)
	materialBody, _ := json.Marshal(map[string]any{"subject_id": "subject:test", "name": "Материал контракта", "content": "Текст материала для поиска", "outcome_ids": []string{outcomeID}})
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", string(materialBody), admin))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", "{"))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", "{", access))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", strings.Repeat(" ", 1400000), admin))
	check("POST", "/auth/refresh", f.request("POST", "/auth/refresh", "", refresh))
	check("POST", "/auth/logout", f.request("POST", "/auth/logout", "", refresh))
	check("GET", "/health", f.request("GET", "/health", ""))
	// Learning plan: a student whose diagnostic has a mistake gets a plan.
	// The teacher uses the HTTP model fixture.
	f.handler = f.app.Handler()
	access, _, learner := f.register(t, "learner@example.edu")
	var learningVariant struct {
		ID string `json:"id"`
	}
	json.Unmarshal(trainingJSON(f, access, "/variants", "contract-learning-variant", `{"subject_id":"subject:test"}`).Body.Bytes(), &learningVariant)
	var learningStart struct {
		ID string `json:"session_id"`
	}
	json.Unmarshal(trainingJSON(f, access, "/diagnostic-sessions", "contract-learning-start", `{"variant_id":"`+learningVariant.ID+`"}`).Body.Bytes(), &learningStart)
	learningDiagnostic, err := f.app.diagnosticStore.Get(t.Context(), learner, learningStart.ID)
	if err != nil {
		t.Fatal(err)
	}
	failedTask := learningDiagnostic.Variant.Competencies[0].Tasks[0]
	if _, _, err = f.app.diagnosticStore.Reserve(t.Context(), learner, learningDiagnostic.ID, "learning-answer", "digest", failedTask.ID, "token"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.app.diagnosticStore.Accept(t.Context(), learner, learningDiagnostic.ID, "token", "learning-answer", "digest", diagnostic.Answer{VariantTaskID: failedTask.ID, CompetencyID: failedTask.CompetencyID, OutcomeID: failedTask.OutcomeID, Role: "main", Score: 0, Task: failedTask}, diagnostic.Transition{Status: diagnostic.StatusActive, CurrentTask: 1}); err != nil {
		t.Fatal(err)
	}
	// Two actually checked gaps keep a second lesson available after retrying
	// the first. Untested siblings no longer enter a learning plan.
	failedBasic := learningDiagnostic.Variant.Competencies[0].Tasks[1]
	if _, _, err = f.app.diagnosticStore.Reserve(t.Context(), learner, learningDiagnostic.ID, "learning-basic", "digest-basic", failedBasic.ID, "token-basic"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.app.diagnosticStore.Accept(t.Context(), learner, learningDiagnostic.ID, "token-basic", "learning-basic", "digest-basic", diagnostic.Answer{VariantTaskID: failedBasic.ID, CompetencyID: failedBasic.CompetencyID, OutcomeID: failedBasic.OutcomeID, Role: "basic", Score: 0, Task: failedBasic}, diagnostic.Transition{Status: diagnostic.StatusCompleted, CurrentCompetency: 1}); err != nil {
		t.Fatal(err)
	}
	learningPath := "/diagnostic-sessions/" + learningDiagnostic.ID + "/learning-plan"
	check("GET", "/diagnostic-sessions/{id}/learning-plan", f.request("GET", learningPath, "", access))
	created := f.request("POST", learningPath, `{"hours":3}`, access)
	check("POST", "/diagnostic-sessions/{id}/learning-plan", created)
	check("POST", "/diagnostic-sessions/{id}/learning-plan", f.request("POST", learningPath, `{"hours":0}`, access))
	check("GET", "/diagnostic-sessions/{id}/learning-plan", f.request("GET", learningPath, "", access))
	var plan struct {
		ID           string `json:"id"`
		CurrentIndex int    `json:"current_index"`
	}
	if err = json.Unmarshal(created.Body.Bytes(), &plan); err != nil || plan.ID == "" {
		t.Fatalf("learning plan: %s", created.Body.String())
	}
	planPath := "/learning-plans/" + plan.ID
	check("GET", "/learning-plans/{id}", f.request("GET", planPath, "", access))
	check("POST", "/learning-plans/{id}/answers", upload(f, planPath+"/answers", "audio", "voice.wav", "audio/wav", wavBytes(), access))
	check("POST", "/learning-plans/{id}/open", f.request("POST", planPath+"/open", "", access))
	check("GET", "/learning-plans/{id}/items/{index}/messages", f.request("GET", fmt.Sprintf("%s/items/%d/messages", planPath, plan.CurrentIndex), "", access))
	check("POST", "/learning-plans/{id}/answers/retry", f.request("POST", planPath+"/answers/retry", "", access))
	// A check that failed leaves the answer waiting; retry checks it again.
	if _, err = f.pool.Exec(t.Context(), `UPDATE learning_plans SET plan_data = jsonb_set(plan_data, '{pending_answer}', '"Ответ"') WHERE id=$1`, plan.ID); err != nil {
		t.Fatal(err)
	}
	check("POST", "/learning-plans/{id}/answers", upload(f, planPath+"/answers", "audio", "voice.wav", "audio/wav", wavBytes(), access))
	check("POST", "/learning-plans/{id}/answers/retry", f.request("POST", planPath+"/answers/retry", "", access))
	// The retried answer passed the topic; the next lesson takes a fresh answer.
	check("POST", "/learning-plans/{id}/open", f.request("POST", planPath+"/open", "", access))
	check("POST", "/learning-plans/{id}/answers", upload(f, planPath+"/answers", "audio", "voice.wav", "audio/wav", wavBytes(), access))

	// An independent plan verifies the JSON path while retaining legacy audio coverage.
	confirmedDiagnostic, err := f.app.diagnosticStore.Get(t.Context(), learner, learningDiagnostic.ID)
	if err != nil {
		t.Fatal(err)
	}
	confirmedDiagnostic.ID = "contract-confirmed-learning"
	confirmedDiagnostic.StartRequestDigest = "confirmed-learning"
	if _, _, err = f.app.diagnosticStore.Create(t.Context(), learner, "confirmed-learning", "confirmed-learning", confirmedDiagnostic); err != nil {
		t.Fatal(err)
	}
	confirmedPlanResponse := f.request("POST", "/diagnostic-sessions/"+confirmedDiagnostic.ID+"/learning-plan", `{"hours":3}`, access)
	confirmedPlan := decodePlan(t, confirmedPlanResponse.Body.Bytes())
	if confirmedPlan.CurrentIndex == nil {
		t.Fatal(confirmedPlanResponse.Body.String())
	}
	confirmedPlanPath := "/learning-plans/" + confirmedPlan.ID
	if opened := f.request("POST", confirmedPlanPath+"/open", "", access); opened.Code != 200 {
		t.Fatal(opened.Body.String())
	}
	messagesResponse := f.request("GET", fmt.Sprintf("%s/items/%d/messages", confirmedPlanPath, *confirmedPlan.CurrentIndex), "", access)
	var messages struct {
		Items []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"items"`
	}
	if err = json.Unmarshal(messagesResponse.Body.Bytes(), &messages); err != nil {
		t.Fatal(err)
	}
	var question string
	for _, message := range messages.Items {
		if message.Kind == "practice" {
			question = message.Text
		}
	}
	ownedPreview := upload(f, "/voice/transcriptions", "audio", "voice.wav", "audio/wav", wavBytes(), access)
	var confirmedPreview struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(ownedPreview.Body.Bytes(), &confirmedPreview); err != nil {
		t.Fatal(err)
	}
	confirmedLearningBody, _ := json.Marshal(map[string]any{"transcription_id": confirmedPreview.ID, "item_index": *confirmedPlan.CurrentIndex, "attempts": 0, "question": question})
	check("POST", "/learning-plans/{id}/answers/transcription", f.request("POST", confirmedPlanPath+"/answers/transcription", string(confirmedLearningBody), access))

	for path, node := range paths {
		implemented := false
		for _, value := range node.(map[string]any) {
			if operation, ok := value.(map[string]any); ok && operation["x-implementation"] == "implemented" {
				implemented = true
			}
		}
		if implemented && !checked[path] {
			t.Errorf("no verified successful response for %s", path)
		}
	}
}

func TestHTTPSCookieLifecycle(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	server := httptest.NewTLSServer(f.handler)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Jar = jar
	send := func(method, path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	response := send("POST", "/auth/register", `{"email":"https@example.edu","password":"`+password+`","display_name":"HTTPS"}`)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 201 || bytes.Contains(body, []byte("access_token")) {
		t.Fatalf("registration: %s", body)
	}
	for _, step := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/auth/me", 200}, {"POST", "/auth/refresh", 200}, {"GET", "/auth/me", 200},
		{"POST", "/auth/logout", 204}, {"GET", "/auth/me", 401},
	} {
		resp := send(step.method, step.path, "")
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != step.status {
			t.Fatalf("cookie lifecycle %s: %d %s", step.path, resp.StatusCode, data)
		}
	}
}

func TestCSVExample(t *testing.T) {
	b, err := os.ReadFile("testdata/example-map.csv")
	if err != nil {
		t.Fatal(err)
	}
	m, err := csvparser.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Outcomes) != 2 || len(m.Tasks) != 4 {
		t.Fatalf("bad example: %+v", m)
	}
}
