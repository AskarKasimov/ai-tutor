package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validBackendYAML = `server:
  listen_address: ":18002"
logging: {level: warn}
voice:
  stt: {url: "http://stt.test/transcribe"}
  tts: {url: "http://tts.test/synthesize", seed: 0, cfg_value: 2, inference_timesteps: 20}
  timeout: 120s
uploads: {max_bytes: 1024}
storage:
  s3: {endpoint: "", region: us-east-1, bucket: audio, path_style: true, timeout: 30s, create_bucket: false}
workers: {audio: {count: 1, poll_interval: 1s}}
tutor:
  enabled: false
  context: {task_max_chars: 16000, history_max_chars: 16000, summary_max_chars: 4000}
course_graph: {url: "http://course-graph:8000", timeout: 0s}
llm_gateway: {address: "gateway.test:8011", timeout: 120s}
geek: {question_limit: 0}
`

func TestLoadConfigYAMLUsesValuesAndSecrets(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "access")
	t.Setenv("BACKEND_S3_SECRET_KEY", "secret")
	t.Setenv("BACKEND_LISTEN_ADDRESS", ":8002") // legacy app env must not override YAML
	cfg, err := LoadConfig(writeYAML(t, validBackendYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != ":18002" || cfg.TTSSeed != 0 || cfg.LearningTutorEnabled || cfg.CourseGraphTimeout != 0 || cfg.GeekMaxQuestions != 0 || cfg.DatabaseURL != "postgres://example/db" {
		t.Fatalf("loaded config = %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidYAML(t *testing.T) {
	cases := []struct{ name, body string }{
		{"missing required", strings.Replace(validBackendYAML, "  timeout: 120s\n", "", 1)},
		{"unknown", validBackendYAML + "extra: true\n"},
		{"duplicate", strings.Replace(validBackendYAML, "server:\n", "server:\nserver:\n", 1)},
		{"multiple documents", validBackendYAML + "---\n" + validBackendYAML},
		{"wrong type", strings.Replace(validBackendYAML, "count: 1", "count: many", 1)},
		{"merge fractional integer", strings.Replace(validBackendYAML, "count: 1", "<<: {count: 1.9}", 1)},
		{"fractional integer", strings.Replace(validBackendYAML, "count: 1", "count: 1.9", 1)},
		{"string boolean", strings.Replace(validBackendYAML, "enabled: false", "enabled: yes", 1)},
		{"boolean string", strings.Replace(validBackendYAML, "bucket: audio", "bucket: false", 1)},
		{"bad duration", strings.Replace(validBackendYAML, "timeout: 120s", "timeout: 0", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
			t.Setenv("BACKEND_S3_ACCESS_KEY", "a")
			t.Setenv("BACKEND_S3_SECRET_KEY", "s")
			if _, err := LoadConfig(writeYAML(t, tc.body)); err == nil {
				t.Fatal("expected YAML error")
			}
		})
	}
}

func TestLoadConfigAllowsIntegerForFloatField(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "a")
	t.Setenv("BACKEND_S3_SECRET_KEY", "s")
	cfg, err := LoadConfig(writeYAML(t, validBackendYAML))
	if err != nil || cfg.TTSCFGValue != 2 {
		t.Fatalf("cfg_value integer: got %v, %v", cfg.TTSCFGValue, err)
	}
}

func TestLoadConfigValidationErrorsUseYAMLPaths(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "private-access")
	t.Setenv("BACKEND_S3_SECRET_KEY", "private-secret")
	badYAML := strings.Replace(validBackendYAML, "timeout: 120s", "timeout: -1s", 1)
	_, err := LoadConfig(writeYAML(t, badYAML))
	if err == nil || !strings.Contains(err.Error(), "voice.timeout") || strings.Contains(err.Error(), "BACKEND_VOICE_TIMEOUT") || strings.Contains(err.Error(), "private-") {
		t.Fatalf("validation error = %v, want safe YAML path", err)
	}
}

func TestLoadConfigMissingBackendSecretsAreNamedWithoutValues(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "")
	t.Setenv("BACKEND_S3_SECRET_KEY", "")
	_, err := LoadConfig(writeYAML(t, validBackendYAML))
	if err == nil || !strings.Contains(err.Error(), "BACKEND_DATABASE_URL") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("safe secret error = %v", err)
	}
}

func TestLoadLLMGatewayConfigDoesNotRequireBackendSecrets(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "")
	t.Setenv("BACKEND_S3_SECRET_KEY", "")
	address, timeout, err := LoadLLMGatewayConfig(writeYAML(t, validBackendYAML))
	if err != nil || address != "gateway.test:8011" || timeout != 120*time.Second {
		t.Fatalf("got %q %s %v", address, timeout, err)
	}
}

func TestLoadConfigRequiresAKnownLogLevel(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "access")
	t.Setenv("BACKEND_S3_SECRET_KEY", "secret")
	cfg, err := LoadConfig(writeYAML(t, validBackendYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != zapcore.WarnLevel {
		t.Fatalf("log level = %s", cfg.LogLevel)
	}
	if _, err := LoadConfig(writeYAML(t, strings.Replace(validBackendYAML, "logging: {level: warn}\n", "", 1))); err == nil || !strings.Contains(err.Error(), "logging.level") {
		t.Fatalf("missing level error = %v", err)
	}
	if _, err := LoadConfig(writeYAML(t, strings.Replace(validBackendYAML, "level: warn", "level: loud", 1))); err == nil || !strings.Contains(err.Error(), "logging.level") {
		t.Fatalf("unknown level error = %v", err)
	}
}

func TestLoadSpeechAPIOptions(t *testing.T) {
	t.Setenv("BACKEND_DATABASE_URL", "postgres://example/db")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "a")
	t.Setenv("BACKEND_S3_SECRET_KEY", "s")
	body := strings.Replace(validBackendYAML, `stt: {url: "http://stt.test/transcribe"}`, `stt: {url: "http://stt.test/transcribe", model: multilingual_large_ctc, longform: true}`, 1)
	body = strings.Replace(body, "inference_timesteps: 20}", "inference_timesteps: 20, use_reference: false, use_prompt: false}", 1)
	cfg, err := LoadConfig(writeYAML(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.STTModel != "multilingual_large_ctc" || !cfg.STTLongform || cfg.TTSUseReference || cfg.TTSUsePrompt {
		t.Fatalf("speech options ignored: %+v", cfg)
	}
}

func TestVoiceNameSeparatesStyleAndModes(t *testing.T) {
	a := &App{cfg: Config{TTSSeed: 607, TTSCFGValue: 2, TTSInferenceTimesteps: 20, TTSUseReference: true, TTSUsePrompt: true}}
	if got := a.voiceName(); got != "seed=607 cfg=2 steps=20 style=none-v1" {
		t.Fatalf("voice name = %q", got)
	}
	a.cfg.TTSUsePrompt = false
	if a.voiceName() == "seed=607 cfg=2 steps=20 style=none-v1" {
		t.Fatal("different mode shares voice name")
	}
}
