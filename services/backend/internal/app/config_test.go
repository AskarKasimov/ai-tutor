package app

import (
	"strings"
	"testing"
	"time"
)

func configuredEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{
		"BACKEND_LISTEN_ADDRESS":          ":8002",
		"BACKEND_DATABASE_URL":            "postgres://example/db",
		"BACKEND_STT_URL":                 "http://stt.test/transcribe",
		"BACKEND_TTS_URL":                 "http://tts.test/synthesize",
		"BACKEND_TTS_SEED":                "17",
		"BACKEND_TTS_CFG_VALUE":           "2",
		"BACKEND_TTS_INFERENCE_TIMESTEPS": "50",
		"BACKEND_VOICE_TIMEOUT":           "120s",
		"BACKEND_MAX_UPLOAD_BYTES":        "26214400",
		"BACKEND_LLM_GATEWAY_ADDR":        "gateway.test:8011",
		"BACKEND_S3_ENDPOINT":             "",
		"BACKEND_S3_REGION":               "us-east-1",
		"BACKEND_S3_BUCKET":               "task-audio-test",
		"BACKEND_S3_ACCESS_KEY":           "test-access",
		"BACKEND_S3_SECRET_KEY":           "test-secret",
		"BACKEND_S3_PATH_STYLE":           "true",
		"BACKEND_S3_TIMEOUT":              "30s",
		"BACKEND_S3_CREATE_BUCKET":        "true",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	return values
}

func TestAudioConfigRequiresEverySetting(t *testing.T) {
	for key := range configuredEnv(t) {
		if key == "BACKEND_TTS_INFERENCE_TIMESTEPS" || key == "BACKEND_S3_ENDPOINT" || key == "BACKEND_S3_REGION" || key == "BACKEND_S3_PATH_STYLE" || key == "BACKEND_S3_TIMEOUT" || key == "BACKEND_S3_CREATE_BUCKET" {
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
	if cfg.STTURL != "http://stt.test/transcribe" || cfg.VoiceTimeout != 120*time.Second || cfg.MaxUploadBytes != 1024 || cfg.S3Bucket != "task-audio-test" {
		t.Fatalf("settings were replaced or ignored: %+v", cfg)
	}
}

func TestAudioConfigRejectsInvalidSettings(t *testing.T) {
	cases := map[string][]string{
		"BACKEND_LISTEN_ADDRESS":          {"bad", ":bad", ":70000"},
		"BACKEND_STT_URL":                 {"relative", "ftp://stt.test", "http://user:password@stt.test"},
		"BACKEND_TTS_URL":                 {"http://tts.test/#fragment"},
		"BACKEND_TTS_SEED":                {"bad", "1.5", "9223372036854775808"},
		"BACKEND_TTS_CFG_VALUE":           {"bad", "-0.1", "5.1", "NaN", "Inf"},
		"BACKEND_TTS_INFERENCE_TIMESTEPS": {"bad", "0", "51", "1.5"},
		"BACKEND_VOICE_TIMEOUT":           {"bad", "0s", "-1s"},
		"BACKEND_MAX_UPLOAD_BYTES":        {"bad", "0", "-1", "26214401"},
		"BACKEND_S3_ENDPOINT":             {"relative", "ftp://s3.test", "http://user:pass@s3.test", "http://s3.test/?x=1", "http://s3.test/#fragment"},
		"BACKEND_S3_BUCKET":               {""},
		"BACKEND_S3_ACCESS_KEY":           {""},
		"BACKEND_S3_SECRET_KEY":           {""},
		"BACKEND_S3_PATH_STYLE":           {"maybe"},
		"BACKEND_S3_TIMEOUT":              {"bad", "0s", "-1s"},
		"BACKEND_LLM_GATEWAY_ADDR":        {"gateway", ":8011", "http://gateway:8011", "gateway:abc", "gateway:0", "gateway:-1", "gateway:65536", "gateway:70000"},
		"BACKEND_LLM_TIMEOUT":             {"bad", "0s", "-1s"},
		"BACKEND_GEEK_QUESTION_LIMIT":     {"bad", "-1"},
		"BACKEND_COURSE_GRAPH_TIMEOUT":    {"bad", "-1s"},
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

func TestConfigDoesNotRequireAPIMode(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_API_MODE", "")
	if _, err := ConfigFromEnv(); err != nil {
		t.Fatalf("model configuration must work without an API mode: %v", err)
	}
}

func TestLegacyMockModeCannotBypassModelSettings(t *testing.T) {
	configuredEnv(t)
	t.Setenv("BACKEND_API_MODE", "mock")
	t.Setenv("BACKEND_STT_URL", "")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "BACKEND_STT_URL") {
		t.Fatalf("got %v, want missing STT setting error", err)
	}
}

func testConfig() Config {
	return Config{
		ListenAddress: ":8002", DatabaseURL: "postgres://example/db",
		STTURL: "http://stt.test/transcribe", TTSURL: "http://tts.test/synthesize",
		TTSSeed: 17, TTSCFGValue: 2, TTSInferenceTimesteps: 50,
		VoiceTimeout: 120 * time.Second, LLMTimeout: 120 * time.Second, LLMGatewayAddress: "gateway.test:8011",
		MaxUploadBytes: 25 * 1024 * 1024,
		S3Region:       "us-east-1", S3Bucket: "task-audio-test", S3AccessKey: "test-access", S3SecretKey: "test-secret",
		S3PathStyle: true, S3Timeout: 30 * time.Second, S3CreateBucket: true,
		AudioWorkers: 1, AudioPollInterval: time.Second, LearningTutorEnabled: true, TutorTaskMaxChars: 16000, TutorHistoryMaxChars: 16000, TutorSummaryMaxChars: 4000,
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
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "https://objects.example.test" || cfg.S3Region != "eu-west-1" || cfg.S3Bucket != "private-audio" || cfg.S3PathStyle || cfg.S3Timeout != 12*time.Second || cfg.S3CreateBucket {
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
	t.Setenv("BACKEND_TTS_INFERENCE_TIMESTEPS", "30")
	if cfg, err = ConfigFromEnv(); err != nil || cfg.TTSInferenceTimesteps != 30 {
		t.Fatalf("explicit steps = %d, %v", cfg.TTSInferenceTimesteps, err)
	}
	t.Setenv("BACKEND_TTS_INFERENCE_TIMESTEPS", "")
	if cfg, err = ConfigFromEnv(); err != nil || cfg.TTSInferenceTimesteps != DefaultTTSInferenceTimesteps {
		t.Fatalf("default steps = %d, %v", cfg.TTSInferenceTimesteps, err)
	}
}

func TestGeekConfigRequiresTheGatewayAndLimitsQuestions(t *testing.T) {
	configuredEnv(t)
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMGatewayAddress != "gateway.test:8011" || cfg.LLMTimeout != 120*time.Second || cfg.GeekMaxQuestions != DefaultGeekQuestionLimit {
		t.Fatalf("geek defaults = %+v", cfg)
	}
	t.Setenv("BACKEND_LLM_GATEWAY_ADDR", "llm-gateway:8011")
	t.Setenv("BACKEND_GEEK_QUESTION_LIMIT", "12")
	if cfg, err = ConfigFromEnv(); err != nil || cfg.LLMGatewayAddress != "llm-gateway:8011" || cfg.GeekMaxQuestions != 12 {
		t.Fatalf("geek settings = %+v, %v", cfg, err)
	}
}

func TestGatewayAddressAcceptsBoundaryPorts(t *testing.T) {
	for _, address := range []string{"llm-gateway:1", "llm-gateway:65535", "10.0.0.5:8011"} {
		configuredEnv(t)
		t.Setenv("BACKEND_LLM_GATEWAY_ADDR", address)
		if _, err := ConfigFromEnv(); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
}

func TestCourseGraphTimeoutDefaultsToNone(t *testing.T) {
	configuredEnv(t)
	cfg, err := ConfigFromEnv()
	if err != nil || cfg.CourseGraphTimeout != 0 {
		t.Fatalf("default course-graph timeout = %v, %v; want 0 (no overall timeout)", cfg.CourseGraphTimeout, err)
	}
	t.Setenv("BACKEND_COURSE_GRAPH_TIMEOUT", "15m")
	if cfg, err = ConfigFromEnv(); err != nil || cfg.CourseGraphTimeout != 15*time.Minute {
		t.Fatalf("course-graph timeout = %v, %v; want 15m", cfg.CourseGraphTimeout, err)
	}
}
