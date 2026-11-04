package modelapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestSynthesizeSendsConfiguredVoiceSettings(t *testing.T) {
	wav := testWAV()
	for _, options := range []TTSOptions{
		{Seed: 17, CFGValue: 2, InferenceTimesteps: 50},
		{Seed: 0, CFGValue: 0, InferenceTimesteps: 1},
		{Seed: 91, CFGValue: 1.5, InferenceTimesteps: 20},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Error("invalid TTS request")
			}
			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			want := map[string]any{"text": "(calm, composed, friendly tone; speak evenly, without irritation or anger) Привет.!!!", "seed": float64(options.Seed), "cfg_value": options.CFGValue, "inference_timesteps": float64(options.InferenceTimesteps)}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("payload = %v, want %v", got, want)
			}
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wav)
		}))
		client := New(server.Client(), "", server.URL, time.Second, options)
		got, err := client.Synthesize(context.Background(), "Привет.")
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, wav) {
			t.Fatal("WAV response changed")
		}
	}
}

func TestSynthesizeAddsCompletionSuffix(t *testing.T) {
	wav := testWAV()
	spokenText := "Не произносите формулу. Объясните, почему аналитическое решение в обычном виде неприменимо"
	want := "(calm, composed, friendly tone; speak evenly, without irritation or anger) " + spokenText + "!!!"
	for _, text := range []string{spokenText, spokenText + "!!!"} {
		t.Run(text, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Text string `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Text != want {
					t.Errorf("text = %q, want %q", payload.Text, want)
				}
				w.Header().Set("Content-Type", "audio/wav")
				_, _ = w.Write(wav)
			}))
			defer server.Close()
			client := New(server.Client(), "", server.URL, time.Second, TTSOptions{})
			got, err := client.Synthesize(context.Background(), text)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, wav) {
				t.Fatal("WAV response changed")
			}
		})
	}
}

func TestRecognizeSendsLongWAVInParts(t *testing.T) {
	const rate = 8000
	frames := make([]byte, rate*45*2)
	for i := 0; i < len(frames); i += 2 {
		frames[i] = 0x40
	}
	data := make([]byte, 44)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+len(frames)))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(frames)))
	data = append(data, frames...)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		part, _ := io.ReadAll(file)
		if seconds := float64(len(part)-44) / 2 / rate; seconds > 20 {
			t.Errorf("part of %.1f s exceeds the short-form limit", seconds)
		}
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{"text": fmt.Sprintf("часть%d", calls)})
	}))
	defer server.Close()
	text, err := New(server.Client(), server.URL, "", time.Second, TTSOptions{}).Recognize(context.Background(), data, "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || text != "часть1 часть2 часть3" {
		t.Fatalf("calls=%d text=%q", calls, text)
	}
}

func testWAV() []byte {
	data := make([]byte, 46)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 38)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 2)
	return data
}

func TestRecognizeUsesServerLongformAndModel(t *testing.T) {
	const rate = 8000
	frames := make([]byte, rate*45*2)
	for i := 0; i < len(frames); i += 2 {
		frames[i] = 0x40
	}
	data := make([]byte, 44)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+len(frames)))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(frames)))
	data = append(data, frames...)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		part, _ := io.ReadAll(file)
		if len(part) != len(data) || r.URL.Query().Get("model") != "multilingual_large_ctc" {
			t.Errorf("wrong request: length=%d query=%v", len(part), r.URL.Query())
		}
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{"text": fmt.Sprintf("часть%d", calls)})
	}))
	defer server.Close()
	text, err := New(server.Client(), server.URL, "", time.Second, TTSOptions{}).WithSTTOptions("multilingual_large_ctc", true).Recognize(context.Background(), data, "audio/wav")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || text != "часть1" {
		t.Fatalf("calls=%d text=%q", calls, text)
	}
}

func TestSynthesizeSendsVoiceMode(t *testing.T) {
	disabled := false
	enabled := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["use_reference"] != false || got["use_prompt"] != true {
			t.Errorf("unexpected voice mode: %v", got)
		}
		_, _ = w.Write(testWAV())
	}))
	defer server.Close()
	_, err := New(server.Client(), "", server.URL, time.Second, TTSOptions{UseReference: &disabled, UsePrompt: &enabled}).Synthesize(context.Background(), "Привет")
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecognizeDistinguishesInvalidModelFromInvalidAudio(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"detail":"Unsupported ASR model: invalid_model"}`, "PROCESSING_UNAVAILABLE"},
		{`{"detail":"Unsupported or invalid audio"}`, "INVALID_AUDIO"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := New(server.Client(), server.URL, "", time.Second, TTSOptions{}).WithSTTOptions("invalid_model", false).Recognize(context.Background(), testWAV(), "audio/wav")
			var got *fault.Error
			if !errors.As(err, &got) || got.Code != tc.code {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}
}
