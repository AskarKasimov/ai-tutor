package ocr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOCRKeepsUsableTextAndExposesDegradedRecognition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"markdown":"Исходный текст","report":{"warnings":["Часть формул не распознана"]}}`))
	}))
	defer server.Close()
	text, err := New(server.URL, time.Second).Convert(context.Background(), "lecture.pdf", []byte("pdf"))
	if text != "Исходный текст" || err == nil || !strings.Contains(err.Error(), "Часть формул") {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestOCRClientWaitsForModelsBeforeClaimAndUsesStableJobID(t *testing.T) {
	var gotJobID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"ready": true, "state": "ready"})
		case "/ocr/convert":
			gotJobID = r.Header.Get("X-Job-ID")
			_ = json.NewEncoder(w).Encode(map[string]any{"markdown": "готово", "report": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, 0)
	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	text, err := client.ConvertJob(context.Background(), "mat_stable", "lecture.pdf", []byte("pdf"))
	if err != nil || text != "готово" || gotJobID != "mat_stable" {
		t.Fatalf("text=%q job=%q err=%v", text, gotJobID, err)
	}
	if client.httpClient.Timeout != 0 {
		t.Fatalf("OCR total timeout = %s, want unlimited", client.httpClient.Timeout)
	}
}

func TestOCRReadinessFalseIsNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": false, "state": "loading"})
	}))
	defer server.Close()
	if err := New(server.URL, 0).Ready(context.Background()); err == nil {
		t.Fatal("expected model loading to remain a wait state")
	}
}
