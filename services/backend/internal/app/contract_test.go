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
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/csvparser"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// Validate actual handler responses using the checked-in OpenAPI 3.1 schemas.
func TestOpenAPIResponses(t *testing.T) {
	f := newFixture(t)
	data, err := os.ReadFile("../../../../api/01-voice/openapi.yaml")
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
	// Normalize YAML number types into JSON types understood by the validator.
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
		ref := schemaNode["$ref"].(string)
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
		if r.URL.Path == "/synthesize" {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wavBytes())
		} else {
			httpx.JSON(w, 200, map[string]any{"text": "Ответ.", "model": "v3", "segments": []any{}})
		}
	}))
	defer provider.Close()
	f.app.cfg.STTURL = provider.URL + "/transcribe/longform"
	f.app.cfg.TTSURL = provider.URL + "/synthesize"
	check("POST", "/voice/transcriptions", upload(f, "/voice/transcriptions", "audio", "answer.wav", "audio/wav", wavBytes(), access))
	check("POST", "/voice/syntheses", f.request("POST", "/voice/syntheses", `{"text":"Вопрос?"}`, access))
	check("POST", "/voice/syntheses", f.request("POST", "/voice/syntheses", `{"text":""}`, access))
	admin := f.admin(t)
	check("POST", "/admin/competency-map/import", upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(mapCSV), admin))
	check("POST", "/auth/refresh", f.request("POST", "/auth/refresh", "", refresh))
	check("POST", "/auth/logout", f.request("POST", "/auth/logout", "", refresh))
	for path := range paths {
		if !checked[path] {
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
	b, err := os.ReadFile("../../../../api/01-voice/example-map.csv")
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

func TestConfigRejectsBadSettings(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "STT_URL", "TTS_URL", "PASSWORD_CHECK_URL", "PROCESSING_TIMEOUT", "AUTH_RATE_LIMIT", "LISTEN_ADDRESS"} {
		t.Setenv(key, "")
	}
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("missing database accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://example/db")
	t.Setenv("PROCESSING_TIMEOUT", "bad")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("bad duration accepted")
	}
	t.Setenv("PROCESSING_TIMEOUT", "1s")
	t.Setenv("AUTH_RATE_LIMIT", "0")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("unlimited auth accepted")
	}
}
