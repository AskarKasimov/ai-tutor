package app

import (
	"strings"
	"testing"
	"time"
)

func configuredEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{
		"BACKEND_API_MODE":                "real",
		"BACKEND_LISTEN_ADDRESS":          ":8002",
		"BACKEND_DATABASE_URL":            "postgres://example/db",
		"BACKEND_STT_URL":                 "http://stt.test/transcribe",
		"BACKEND_TTS_URL":                 "http://tts.test/synthesize",
		"BACKEND_TTS_SEED":                "17",
		"BACKEND_TTS_CFG_VALUE":           "2",
		"BACKEND_TTS_INFERENCE_TIMESTEPS": "50",
		"BACKEND_ASSESSMENT_BASE_URL":     "http://assessment.test/v1",
		"BACKEND_ASSESSMENT_MODEL":        "test-model",
		"BACKEND_TASKGEN_BASE_URL":        "http://generation.test/v1",
		"BACKEND_TASKGEN_MODEL":           "generation-model",
		"BACKEND_TASKGEN_TIMEOUT":         "45s",
		"BACKEND_VOICE_TIMEOUT":           "120s",
		"BACKEND_ASSESSMENT_TIMEOUT":      "90s",
		"BACKEND_MAX_UPLOAD_BYTES":        "26214400",
		"BACKEND_S3_ENDPOINT":             "",
		"BACKEND_S3_REGION":               "us-east-1",
		"BACKEND_S3_BUCKET":               "task-audio-test",
		"BACKEND_S3_ACCESS_KEY":           "test-access",
		"BACKEND_S3_SECRET_KEY":           "test-secret",
		"BACKEND_S3_PATH_STYLE":           "true",
		"BACKEND_S3_TIMEOUT":              "30s",
		"BACKEND_S3_CREATE_BUCKET":        "true",
		"BACKEND_AUDIO_WORKERS":           "1",
		"BACKEND_AUDIO_POLL_INTERVAL":     "1s",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	return values
}

func TestAudioConfigRequiresEverySetting(t *testing.T) {
	for key := range configuredEnv(t) {
		if key == "BACKEND_S3_ENDPOINT" || key == "BACKEND_S3_REGION" || key == "BACKEND_S3_PATH_STYLE" || key == "BACKEND_S3_TIMEOUT" || key == "BACKEND_S3_CREATE_BUCKET" || key == "BACKEND_AUDIO_WORKERS" || key == "BACKEND_AUDIO_POLL_INTERVAL" {
			continue
		}
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

func TestAudioConfigLoadsExplicitSettings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_MAX_UPLOAD_BYTES", "1024")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.STTURL != "http://stt.test/transcribe" || cfg.AssessmentModel != "test-model" || cfg.MaxUploadBytes != 1024 || cfg.TaskgenBaseURL != "http://generation.test/v1" || cfg.TaskgenModel != "generation-model" || cfg.TaskgenTimeout != 45*time.Second || cfg.S3Bucket != "task-audio-test" || cfg.AudioWorkers != 1 || cfg.AudioPollInterval != time.Second {
		t.Fatalf("settings were replaced or ignored: %+v", cfg)
	}
}

func TestAudioConfigRejectsInvalidSettings(t *testing.T) {
	cases := map[string][]string{
		"BACKEND_API_MODE":                {"preview", "REAL", "dev"},
		"BACKEND_LISTEN_ADDRESS":          {"bad", ":bad", ":70000"},
		"BACKEND_STT_URL":                 {"relative", "ftp://stt.test", "http://user:password@stt.test"},
		"BACKEND_TTS_URL":                 {"http://tts.test/#fragment"},
		"BACKEND_TTS_SEED":                {"bad", "1.5", "9223372036854775808"},
		"BACKEND_TTS_CFG_VALUE":           {"bad", "-0.1", "5.1", "NaN", "Inf"},
		"BACKEND_TTS_INFERENCE_TIMESTEPS": {"bad", "0", "51", "1.5"},
		"BACKEND_ASSESSMENT_BASE_URL":     {"relative"},
		"BACKEND_TASKGEN_BASE_URL":        {"relative", "ftp://generation.test", "http://user:pass@generation.test", "http://generation.test/#fragment"},
		"BACKEND_TASKGEN_TIMEOUT":         {"bad", "0s", "-1s"},
		"BACKEND_VOICE_TIMEOUT":           {"bad", "0s", "-1s"},
		"BACKEND_ASSESSMENT_TIMEOUT":      {"bad", "0s"},
		"BACKEND_MAX_UPLOAD_BYTES":        {"bad", "0", "-1", "26214401"},
		"BACKEND_S3_ENDPOINT":             {"relative", "ftp://s3.test", "http://user:pass@s3.test", "http://s3.test/?x=1", "http://s3.test/#fragment"},
		"BACKEND_S3_BUCKET":               {""},
		"BACKEND_S3_ACCESS_KEY":           {""},
		"BACKEND_S3_SECRET_KEY":           {""},
		"BACKEND_S3_PATH_STYLE":           {"maybe"},
		"BACKEND_S3_TIMEOUT":              {"bad", "0s", "-1s"},
		"BACKEND_AUDIO_WORKERS":           {"bad", "0", "-1"},
		"BACKEND_AUDIO_POLL_INTERVAL":     {"bad", "0s", "-1s"},
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
	for _, key := range []string{"BACKEND_STT_URL", "BACKEND_TTS_URL", "BACKEND_TTS_SEED", "BACKEND_TTS_CFG_VALUE", "BACKEND_TTS_INFERENCE_TIMESTEPS", "BACKEND_ASSESSMENT_BASE_URL", "BACKEND_ASSESSMENT_MODEL", "BACKEND_TASKGEN_BASE_URL", "BACKEND_TASKGEN_MODEL", "BACKEND_TASKGEN_TIMEOUT"} {
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
		TTSSeed: 17, TTSCFGValue: 2, TTSInferenceTimesteps: 50,
		AssessmentBaseURL: "http://assessment.test/v1", AssessmentModel: "test-model",
		TaskgenBaseURL: "http://generation.test/v1", TaskgenModel: "generation-model", TaskgenTimeout: 45 * time.Second,
		VoiceTimeout: 120 * time.Second, AssessmentTimeout: 90 * time.Second,
		MaxUploadBytes: 25 * 1024 * 1024,
		S3Region:       "us-east-1", S3Bucket: "task-audio-test", S3AccessKey: "test-access", S3SecretKey: "test-secret",
		S3PathStyle: true, S3Timeout: 30 * time.Second, S3CreateBucket: true, AudioWorkers: 1, AudioPollInterval: time.Second,
	}
}

func TestAudioConfigLoadsCustomS3Settings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_S3_ENDPOINT", "https://objects.example.test")
	t.Setenv("BACKEND_S3_REGION", "eu-west-1")
	t.Setenv("BACKEND_S3_BUCKET", "private-audio")
	t.Setenv("BACKEND_S3_PATH_STYLE", "false")
	t.Setenv("BACKEND_S3_TIMEOUT", "12s")
	t.Setenv("BACKEND_S3_CREATE_BUCKET", "false")
	t.Setenv("BACKEND_AUDIO_WORKERS", "3")
	t.Setenv("BACKEND_AUDIO_POLL_INTERVAL", "250ms")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "https://objects.example.test" || cfg.S3Region != "eu-west-1" || cfg.S3Bucket != "private-audio" || cfg.S3PathStyle || cfg.S3Timeout != 12*time.Second || cfg.S3CreateBucket || cfg.AudioWorkers != 3 || cfg.AudioPollInterval != 250*time.Millisecond {
		t.Fatalf("custom storage config = %+v", cfg)
	}
}

func TestConfigLoadsTTSSettings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_TTS_SEED", "0")
	t.Setenv("BACKEND_TTS_CFG_VALUE", "1.5")
	t.Setenv("BACKEND_TTS_INFERENCE_TIMESTEPS", "20")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTSSeed != 0 || cfg.TTSCFGValue != 1.5 || cfg.TTSInferenceTimesteps != 20 {
		t.Fatalf("TTS settings ignored: %+v", cfg)
	}
}
