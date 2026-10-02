package app

import (
	"strings"
	"testing"
	"time"
)

func configuredEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{
		"LISTEN_ADDRESS":      ":8002",
		"DATABASE_URL":        "postgres://example/db",
		"STT_URL":             "http://stt.test/transcribe",
		"TTS_URL":             "http://tts.test/synthesize",
		"ASSESSMENT_BASE_URL": "http://assessment.test/v1",
		"ASSESSMENT_MODEL":    "test-model",
		"PROCESSING_TIMEOUT":  "120s",
		"ASSESSMENT_TIMEOUT":  "90s",
		"MAX_UPLOAD_BYTES":    "26214400",
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
	t.Setenv("MAX_UPLOAD_BYTES", "1024")
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
		"LISTEN_ADDRESS":      {"bad", ":bad", ":70000"},
		"STT_URL":             {"relative", "ftp://stt.test", "http://user:password@stt.test"},
		"TTS_URL":             {"http://tts.test/#fragment"},
		"ASSESSMENT_BASE_URL": {"relative"},
		"PROCESSING_TIMEOUT":  {"bad", "0s", "-1s"},
		"ASSESSMENT_TIMEOUT":  {"bad", "0s"},
		"MAX_UPLOAD_BYTES":    {"bad", "0", "-1", "26214401"},
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

func testConfig() Config {
	return Config{
		ListenAddress: ":8002", DatabaseURL: "postgres://example/db",
		STTURL: "http://stt.test/transcribe", TTSURL: "http://tts.test/synthesize",
		AssessmentBaseURL: "http://assessment.test/v1", AssessmentModel: "test-model",
		ProcessingTimeout: 120 * time.Second, AssessmentTimeout: 90 * time.Second,
		MaxUploadBytes: 25 * 1024 * 1024,
	}
}
