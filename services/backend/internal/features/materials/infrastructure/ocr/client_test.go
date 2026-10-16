package ocr

import (
	"context"
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
