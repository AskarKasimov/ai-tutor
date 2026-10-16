package app

import (
	"bytes"
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
		default:
			t.Errorf("unexpected provider path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	f.app.cfg.STTURL = provider.URL + "/transcribe"
	f.app.cfg.TTSURL = provider.URL + "/synthesize"
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
	// Upload and list are planned with week and kind; the old handlers stay
	// unchecked until stream A implements them.
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
	check("GET", "/admin/subjects/{subject_id}/materials/{material_id}/raw", f.request("GET", "/admin/subjects/subject:test/materials/"+matID+"/raw", "", admin))
	check("DELETE", "/admin/subjects/{subject_id}/materials/{material_id}", f.request("DELETE", "/admin/subjects/subject:test/materials/"+matID, "", admin))
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
