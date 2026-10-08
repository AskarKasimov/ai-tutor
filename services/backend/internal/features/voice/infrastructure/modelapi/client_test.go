package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	voicemock "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/mock"
)

func TestSynthesizeSendsConfiguredVoiceSettings(t *testing.T) {
	wav, err := (voicemock.Client{}).Synthesize(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
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
			want := map[string]any{"text": "Привет.", "seed": float64(options.Seed), "cfg_value": options.CFGValue, "inference_timesteps": float64(options.InferenceTimesteps)}
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
