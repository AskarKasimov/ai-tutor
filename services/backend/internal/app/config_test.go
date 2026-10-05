package app

import (
	"strings"
	"testing"
	"time"
)

func configuredEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{
		"BACKEND_API_MODE":            "real",
		"BACKEND_LISTEN_ADDRESS":      ":8002",
		"BACKEND_DATABASE_URL":        "postgres://example/db",
		"BACKEND_STT_URL":             "http://stt.test/transcribe",
		"BACKEND_TTS_URL":             "http://tts.test/synthesize",
		"BACKEND_ASSESSMENT_BASE_URL": "http://assessment.test/v1",
		"BACKEND_ASSESSMENT_MODEL":    "test-model",
		"BACKEND_VOICE_TIMEOUT":       "120s",
		"BACKEND_ASSESSMENT_TIMEOUT":  "90s",
		"BACKEND_MAX_UPLOAD_BYTES":    "26214400",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	return values
}

func TestConfigRequiresEverySetting(t *testing.T) {
	for key := range configuredEnv(t) {
		t.Run(key, func(t *testing.T) {
			configuredEnv(t)
			for _, value := range []string{"", "   "} {
				t.Setenv(key, value)
				if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("%s=%q: got %v, want missing-setting error", key, value, err)
				}
			}
		})
	}
}

func TestConfigLoadsExplicitSettings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_MAX_UPLOAD_BYTES", "1024")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.STTURL != "http://stt.test/transcribe" || cfg.AssessmentModel != "test-model" || cfg.MaxUploadBytes != 1024 {
		t.Fatalf("settings were replaced or ignored: %+v", cfg)
	}
}

func TestConfigRejectsInvalidSettings(t *testing.T) {
	cases := map[string][]string{
		"BACKEND_API_MODE":            {"preview", "REAL", "dev"},
		"BACKEND_LISTEN_ADDRESS":      {"bad", ":bad", ":70000"},
		"BACKEND_STT_URL":             {"relative", "ftp://stt.test", "http://user:password@stt.test"},
		"BACKEND_TTS_URL":             {"http://tts.test/#fragment"},
		"BACKEND_ASSESSMENT_BASE_URL": {"relative"},
		"BACKEND_VOICE_TIMEOUT":       {"bad", "0s", "-1s"},
		"BACKEND_ASSESSMENT_TIMEOUT":  {"bad", "0s"},
		"BACKEND_MAX_UPLOAD_BYTES":    {"bad", "0", "-1", "26214401"},
	}
	for key, values := range cases {
		for _, value := range values {
			t.Run(key+"/"+value, func(t *testing.T) {
				configuredEnv(t)
				t.Setenv(key, value)
				if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("got %v, want %s validation error", err, key)
				}
			})
		}
	}
}

func TestMockConfigDoesNotRequireModelSettings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_API_MODE", "mock")
	for _, key := range []string{"BACKEND_STT_URL", "BACKEND_TTS_URL", "BACKEND_ASSESSMENT_BASE_URL", "BACKEND_ASSESSMENT_MODEL"} {
		t.Setenv(key, "")
	}
	if _, err := ConfigFromEnv(); err != nil {
		t.Fatalf("mock mode must work without model settings: %v", err)
	}
}

func TestMockConfigStillRequiresDatabase(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_API_MODE", "mock")
	t.Setenv("BACKEND_DATABASE_URL", "")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "BACKEND_DATABASE_URL") {
		t.Fatalf("got %v, want missing database error", err)
	}
}

func testConfig() Config {
	return Config{
		APIMode:       "real",
		ListenAddress: ":8002", DatabaseURL: "postgres://example/db",
		STTURL: "http://stt.test/transcribe", TTSURL: "http://tts.test/synthesize",
		AssessmentBaseURL: "http://assessment.test/v1", AssessmentModel: "test-model",
		ProcessingTimeout: 120 * time.Second, AssessmentTimeout: 90 * time.Second,
		MaxUploadBytes: 25 * 1024 * 1024,
	}
}
