package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise production model adapters through HTTP, with deterministic responses
// owned by the tests rather than an offline application mode.
func configureFixtureModels(t *testing.T, cfg *Config) {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/transcribe":
			_ = json.NewEncoder(w).Encode(map[string]string{"text": "Ответ ученика."})
		case "/synthesize":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wavBytes())
		default:
			t.Errorf("unexpected model path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	for _, setting := range []struct {
		value  *string
		suffix string
	}{
		{&cfg.STTURL, "/transcribe"}, {&cfg.TTSURL, "/synthesize"},
	} {
		if strings.Contains(*setting.value, ".test") {
			*setting.value = provider.URL + setting.suffix
		}
	}
}
