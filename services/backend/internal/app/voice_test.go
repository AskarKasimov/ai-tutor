package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

func wavBytes() []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(38))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(2))
	b.Write([]byte{0, 0})
	return b.Bytes()
}

func TestVoiceTranscriptionPersistsOwnerAndUsesLongform(t *testing.T) {
	f := newFixture(t)
	access, _, id := f.register(t, "voice@example.edu")
	wav := wavBytes()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transcribe/longform" || r.Method != "POST" {
			t.Errorf("wrong STT path: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if !bytes.Equal(data, wav) {
			t.Error("audio changed")
			w.WriteHeader(500)
			return
		}
		httpx.JSON(w, 200, map[string]any{"text": "Ответ ученика.", "model": "v3_e2e_rnnt", "segments": []any{}})
	}))
	defer provider.Close()
	f.app.cfg.STTURL = provider.URL + "/transcribe/longform"
	w := upload(f, "/voice/transcriptions", "audio", "voice.wav", "audio/wav", wav, access)
	if w.Code != 200 {
		t.Fatalf("STT: %d %s", w.Code, w.Body.String())
	}
	var tr voicehttp.Transcription
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	var owner, text string
	var created int64
	if err := f.pool.QueryRow(context.Background(), "SELECT user_id,text,created_at FROM transcriptions WHERE id=$1", tr.ID).Scan(&owner, &text, &created); err != nil {
		t.Fatal(err)
	}
	if owner != id || text != "Ответ ученика." || created != 1790762400 || tr.CreatedAt != created {
		t.Fatal("wrong stored transcription")
	}
	if strings.Contains(w.Body.String(), "user_id") || strings.Contains(w.Body.String(), "segments") {
		t.Fatal("internal fields leaked")
	}
}

func TestVoiceSynthesisWAVAndUnicodeValidation(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "tts@example.edu")
	wav := wavBytes()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthesize" {
			t.Error("wrong TTS path")
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req) != 1 || req["text"] != strings.Repeat("я", 500) {
			t.Error("provider payload changed/leaked settings")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer provider.Close()
	f.app.cfg.TTSURL = provider.URL + "/synthesize"
	w := f.request("POST", "/voice/syntheses", `{"text":"`+strings.Repeat("я", 500)+`"}`, access)
	if w.Code != 200 || w.Header().Get("Content-Type") != "audio/wav" || !bytes.Equal(w.Body.Bytes(), wav) {
		t.Fatalf("TTS: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{`{"text":""}`, `{"text":"   "}`, `{"text":"` + strings.Repeat("я", 501) + `"}`, `{"text":"x","seed":1}`, `{"text":null}`} {
		requireCode(t, f.request("POST", "/voice/syntheses", body, access), 422, "VALIDATION_ERROR")
	}
}

func TestVoiceInputLimitsAndMultipart(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "limits@example.edu")
	const path = "/voice/transcriptions"
	requireCode(t, upload(f, path, "audio", "voice.wav", "audio/wav", wavBytes(), nil), 401, "UNAUTHORIZED")
	requireCode(t, upload(f, path, "audio", "voice.wav", "audio/wav", nil, access), 422, "INVALID_AUDIO")
	requireCode(t, upload(f, path, "audio", "voice.mp3", "audio/mpeg", []byte("ID3"), access), 415, "UNSUPPORTED_AUDIO_FORMAT")
	requireCode(t, upload(f, path, "audio", "voice.wav", "audio/wav", []byte("not a wave"), access), 422, "INVALID_AUDIO")
	requireCode(t, upload(f, path, "file", "voice.wav", "audio/wav", wavBytes(), access), 422, "VALIDATION_ERROR")
	// Mislabelled codec metadata must survive multipart parsing and be rejected.
	ogg := append(append([]byte("OggS"), make([]byte, 23)...), []byte("OpusHead")...)
	requireCode(t, upload(f, path, "audio", "voice.ogg", "audio/ogg; codecs=vorbis", ogg, access), 415, "UNSUPPORTED_AUDIO_FORMAT")
	f.app.cfg.MaxUploadBytes = 20
	requireCode(t, upload(f, path, "audio", "voice.wav", "audio/wav", wavBytes(), access), 413, "UPLOAD_TOO_LARGE")
}

func TestVoiceProviderFailuresNeverPersist(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "errors@example.edu")
	for _, tc := range []struct {
		status int
		body   string
		want   int
		code   string
	}{
		{422, `{"detail":"Unsupported or invalid audio"}`, 422, "INVALID_AUDIO"},
		{415, `{}`, 415, "UNSUPPORTED_AUDIO_FORMAT"},
		{413, `{}`, 413, "UPLOAD_TOO_LARGE"},
		{503, `{}`, 503, "PROCESSING_UNAVAILABLE"},
		{500, `{"detail":"private model exception"}`, 502, "TRANSCRIPTION_FAILED"},
		{200, `{"text":"   ","model":"x","segments":[]}`, 422, "NO_SPEECH_DETECTED"},
		{200, `{"text":null}`, 502, "TRANSCRIPTION_FAILED"},
		{200, `not json`, 502, "TRANSCRIPTION_FAILED"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		f.app.cfg.STTURL = server.URL + "/transcribe/longform"
		w := upload(f, "/voice/transcriptions", "audio", "voice.wav", "audio/wav", wavBytes(), access)
		requireCode(t, w, tc.want, tc.code)
		if strings.Contains(w.Body.String(), "private model") {
			t.Fatal("provider error leaked")
		}
		server.Close()
	}
	var count int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM transcriptions").Scan(&count)
	if count != 0 {
		t.Fatal("failed transcriptions saved")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	f.app.cfg.STTURL = server.URL + "/transcribe/longform"
	f.app.cfg.TTSURL = server.URL + "/synthesize"
	f.app.cfg.ProcessingTimeout = 20 * time.Millisecond
	requireCode(t, upload(f, "/voice/transcriptions", "audio", "voice.wav", "audio/wav", wavBytes(), access), 504, "PROCESSING_TIMEOUT")
	requireCode(t, f.request("POST", "/voice/syntheses", `{"text":"Вопрос"}`, access), 504, "PROCESSING_TIMEOUT")
	server.Close()
	requireCode(t, upload(f, "/voice/transcriptions", "audio", "voice.wav", "audio/wav", wavBytes(), access), 503, "PROCESSING_UNAVAILABLE")
	requireCode(t, f.request("POST", "/voice/syntheses", `{"text":"Вопрос"}`, access), 503, "PROCESSING_UNAVAILABLE")
}

func TestVoiceRejectsInvalidTTSOutput(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "badtts@example.edu")
	badFormat := wavBytes()
	for i := 32; i < 36; i++ {
		badFormat[i] = 0
	}
	for _, tc := range []struct {
		status int
		body   []byte
		want   int
		code   string
	}{
		{200, []byte("not WAV"), 502, "SYNTHESIS_FAILED"},
		{200, badFormat, 502, "SYNTHESIS_FAILED"},
		{500, []byte("private error"), 502, "SYNTHESIS_FAILED"},
		{503, nil, 503, "PROCESSING_UNAVAILABLE"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/wav")
			w.WriteHeader(tc.status)
			_, _ = w.Write(tc.body)
		}))
		f.app.cfg.TTSURL = server.URL + "/synthesize"
		requireCode(t, f.request("POST", "/voice/syntheses", `{"text":"Вопрос"}`, access), tc.want, tc.code)
		server.Close()
	}
}
