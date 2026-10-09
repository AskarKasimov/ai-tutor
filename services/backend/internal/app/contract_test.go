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
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/csvparser"
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
	register := f.request("POST", "/auth/register", `{"email":"contract@example.edu","password":"`+password+`"}`)
	check("POST", "/auth/register", register)
	cookies := register.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatal("missing cookies")
	}
	access, refresh := cookies[0], cookies[1]
	check("GET", "/auth/me", f.request("GET", "/auth/me", "", access))
	check("GET", "/auth/me", f.request("GET", "/auth/me", ""))
	check("POST", "/auth/login", f.request("POST", "/auth/login", `{"email":"contract@example.edu","password":"`+password+`"}`))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/synthesize":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wavBytes())
		case "/transcribe":
			httpx.JSON(w, 200, map[string]any{"text": "Ответ.", "model": "v3", "segments": []any{}})
		case "/chat/completions":
			var request struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			content := `{"score":2,"verdict":"correct","criterion_results":[{"key":"answer_correctness","satisfied":true,"explanation":"Тип задачи назван правильно."},{"key":"instruction_following","satisfied":true,"explanation":"Выбор объяснён через два дискретных класса."}],"feedback":["Верно.","Оба критерия выполнены.","Закрепите тему."]}`
			if len(request.Messages) > 0 && strings.Contains(request.Messages[0].Content, "Ты создаёшь одно учебное задание") {
				content = `{"question":"Новое задание","options":[],"voice_instruction":"Ответьте.","reference_answer":"Ответ","criteria":""}`
			}
			httpx.JSON(w, 200, map[string]any{"choices": []any{map[string]any{
				"message":       map[string]any{"content": content},
				"finish_reason": "stop",
			}}})
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
	check("POST", "/admin/competency-map/import", upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(mapCSV), admin))
	if variantImport := upload(f, "/admin/competency-map/import", "file", "variant.csv", "text/csv", variantMapCSV(t), admin); variantImport.Code != http.StatusOK {
		t.Fatalf("variant map import: %d %s", variantImport.Code, variantImport.Body.String())
	}
	variantCreateRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(""))
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
	f.app.cfg.APIMode = "mock"
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
	f.app.cfg.APIMode = "real"
	check("GET", "/variants", f.request("GET", "/variants", "", access))
	check("GET", "/variants/{id}", f.request("GET", "/variants/"+createdVariant.ID, "", access))
	check("GET", "/variants/{id}/tasks/{task_id}", f.request("GET", "/variants/"+createdVariant.ID+"/tasks/"+createdVariant.Competencies[0].Main.ID, "", access))
	var outcomeID, taskID string
	if err := f.pool.QueryRow(context.Background(), `SELECT outcome.id, task.id FROM outcomes outcome JOIN tasks task ON task.outcome_id=outcome.id ORDER BY task.id LIMIT 1`).Scan(&outcomeID, &taskID); err != nil {
		t.Fatal(err)
	}
	check("GET", "/tasks", f.request("GET", "/tasks", "", access))
	check("GET", "/competency-map", f.request("GET", "/competency-map", "", access))
	check("GET", "/tasks", f.request("GET", "/tasks?importance=6", "", access))
	check("GET", "/tasks/{id}", f.request("GET", "/tasks/"+taskID, "", access))
	requestBody, _ := json.Marshal(map[string]string{"outcome_id": outcomeID})
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
	materialBody, _ := json.Marshal(map[string]any{"name": "Материал контракта", "content": "Текст материала для поиска", "outcome_ids": []string{outcomeID}})
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", string(materialBody), admin))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", "{"))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", "{", access))
	check("POST", "/admin/materials", f.request("POST", "/admin/materials", strings.Repeat(" ", 1400000), admin))
	check("POST", "/auth/refresh", f.request("POST", "/auth/refresh", "", refresh))
	check("POST", "/auth/logout", f.request("POST", "/auth/logout", "", refresh))
	check("GET", "/health", f.request("GET", "/health", ""))
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
	response := send("POST", "/auth/register", `{"email":"https@example.edu","password":"`+password+`"}`)
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
