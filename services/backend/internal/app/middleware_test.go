package app

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareDoesNotInterpretOrigin(t *testing.T) {
	a := &App{cfg: testConfig()}
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"} {
		for _, source := range []string{"", "null", "https://frontend.example", "https://another.example"} {
			t.Run(method+"/"+source, func(t *testing.T) {
				called := false
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				})
				req := httptest.NewRequest(method, "/auth/logout", nil)
				if source != "" {
					req.Header.Set("Origin", source)
				}
				if method == "OPTIONS" {
					req.Header.Set("Access-Control-Request-Method", "POST")
					req.Header.Set("Access-Control-Request-Headers", "content-type")
				}
				w := httptest.NewRecorder()
				a.middleware(next).ServeHTTP(w, req)
				if !called || w.Code != http.StatusNoContent {
					t.Fatalf("request did not reach handler: called=%t status=%d", called, w.Code)
				}
				for header := range w.Header() {
					if strings.HasPrefix(header, "Access-Control-") {
						t.Errorf("unexpected CORS header: %s", header)
					}
				}
				if w.Header().Get("Vary") != "" {
					t.Error("unexpected origin-dependent response")
				}
			})
		}
	}
}

func TestMiddlewareLearningAudioUploadLimits(t *testing.T) {
	a := &App{cfg: testConfig()}
	a.cfg.MaxUploadBytes = 64 * 1024
	paths := []string{
		"/learn/diagnostic-sessions/diag/answers",
		"/learn/training-sessions/train/answers",
		"/learn/learning-plans/plan/answers",
		"/learn/training-sessions/train/exercises/ex/chat/turns",
		"/voice/transcriptions",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body, contentType := multipartAudio(t, 64*1024)
			var readErr error
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reader, err := r.MultipartReader()
				if err != nil {
					readErr = err
					return
				}
				for {
					part, err := reader.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						readErr = err
						return
					}
					_, readErr = io.Copy(io.Discard, part)
					part.Close()
					if readErr != nil {
						return
					}
				}
			})
			req := httptest.NewRequest(http.MethodPost, path, body)
			req.Header.Set("Content-Type", contentType)
			a.middleware(next).ServeHTTP(httptest.NewRecorder(), req)
			if readErr != nil {
				t.Fatalf("64 KiB upload rejected: %v", readErr)
			}
		})
	}
	t.Run("over configured upload limit", func(t *testing.T) {
		body, contentType := multipartAudio(t, 128*1024)
		var readErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, readErr = io.Copy(io.Discard, r.Body)
		})
		req := httptest.NewRequest(http.MethodPost, paths[0], body)
		req.Header.Set("Content-Type", contentType)
		a.middleware(next).ServeHTTP(httptest.NewRecorder(), req)
		if readErr == nil {
			t.Fatal("over-limit multipart body was accepted")
		}
	})
}

func TestMiddlewareKeepsSmallLimitsAndAllowsLearningMaterialImport(t *testing.T) {
	a := &App{cfg: testConfig()}
	for _, tc := range []struct {
		name, method, path string
		bodySize           int
	}{
		{"json route", http.MethodPost, "/learn/diagnostic-sessions", 40 * 1024},
		{"wrong method", http.MethodGet, "/learn/diagnostic-sessions/diag/answers", 40 * 1024},
		{"voice wrong method", http.MethodGet, "/voice/transcriptions", 40 * 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var readErr error
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, readErr = io.Copy(io.Discard, r.Body) })
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(bytes.Repeat([]byte("x"), tc.bodySize)))
			a.middleware(next).ServeHTTP(httptest.NewRecorder(), req)
			if readErr == nil {
				t.Fatal("body larger than the default limit was accepted")
			}
		})
	}
	t.Run("learning materials text import", func(t *testing.T) {
		var readErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, readErr = io.Copy(io.Discard, r.Body) })
		body := `{"content":"` + strings.Repeat(`\ud83d\ude00`, 100000) + `"}`
		var got []byte
		req := httptest.NewRequest(http.MethodPost, "/learn/admin/materials", strings.NewReader(body))
		next = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, readErr = io.ReadAll(r.Body)
		})
		a.middleware(next).ServeHTTP(httptest.NewRecorder(), req)
		if readErr != nil {
			t.Fatalf("100,000-character text import rejected: %v", readErr)
		}
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(got, &payload); err != nil || len([]rune(payload.Content)) != 100000 {
			t.Fatalf("100,000-rune escaped JSON import did not survive transport: runes=%d err=%v", len([]rune(payload.Content)), err)
		}
	})
}

func TestLearningAudioUploadRouteIsExact(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/learn/diagnostic-sessions/diag/answers", true},
		{"/learn/training-sessions/train/answers", true},
		{"/learn/learning-plans/plan/answers", true},
		{"/learn/training-sessions/train/exercises/ex/chat/turns", true},
		{"/learn/diagnostic-sessions//answers", false},
		{"/learn/diagnostic-sessions/diag/answers/", false},
		{"/learn/diagnostic-sessions/diag/answers/extra", false},
		{"/learn/training-sessions/train/answers/reset", false},
		{"/learn/learning-plans/plan/answers/retry", false},
	} {
		if got := isLearningAudioUpload(tc.path); got != tc.want {
			t.Errorf("isLearningAudioUpload(%q)=%t, want %t", tc.path, got, tc.want)
		}
	}
}

func multipartAudio(t *testing.T, size int) (*bytes.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("audio", "answer.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("a"), size)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(body.Bytes()), w.FormDataContentType()
}
