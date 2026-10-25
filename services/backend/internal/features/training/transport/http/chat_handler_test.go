package traininghttp

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

func TestParseChatInputJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"revision":2,"kind":"check","text":"answer"}`))
	r.Header.Set("Content-Type", "application/json")
	got, err := parseChatInput(httptest.NewRecorder(), r, 1024, "request-key", "exercise")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || got.Kind != training.ChatKindCheck || got.Source != training.AnswerSourceText || got.Text != "answer" {
		t.Fatalf("unexpected parsed input: %#v", got)
	}
}

func TestParseChatInputJSONRejectsUnknownFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"revision":0,"kind":"message","text":"hi","role":"system"}`))
	r.Header.Set("Content-Type", "application/json")
	if _, err := parseChatInput(httptest.NewRecorder(), r, 1024, "key", "exercise"); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestParseChatInputMultipartAudio(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("revision", "4")
	_ = w.WriteField("kind", "message")
	part, err := w.CreateFormFile("audio", "voice.webm")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("RIFFxxxxWAVE"))
	_ = w.Close()
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	got, err := parseChatInput(httptest.NewRecorder(), r, 1024, "key", "exercise")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != training.AnswerSourceVoice || got.Revision != 4 || got.Kind != training.ChatKindMessage || len(got.Audio) == 0 {
		t.Fatalf("unexpected parsed input: %#v", got)
	}
}
