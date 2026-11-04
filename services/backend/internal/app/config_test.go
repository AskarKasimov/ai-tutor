package app

import (
	"math"
	"strings"
	"testing"
	"time"
)

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

func TestConfigValidateAcceptsAValidConfig(t *testing.T) {
	if err := testConfig().validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigValidateRejectsInvalidSettings(t *testing.T) {
	text := func(set func(*Config, string), values ...string) []func(*Config) {
		out := []func(*Config){}
		for _, value := range values {
			out = append(out, func(c *Config) { set(c, value) })
		}
		return out
	}
	duration := func(set func(*Config, time.Duration), values ...time.Duration) []func(*Config) {
		out := []func(*Config){}
		for _, value := range values {
			out = append(out, func(c *Config) { set(c, value) })
		}
		return out
	}
	cases := map[string][]func(*Config){
		"BACKEND_LISTEN_ADDRESS": text(func(c *Config, v string) { c.ListenAddress = v }, "bad", ":bad", ":70000"),
		"BACKEND_DATABASE_URL":   text(func(c *Config, v string) { c.DatabaseURL = v }, "", "   "),
		"BACKEND_STT_URL":        text(func(c *Config, v string) { c.STTURL = v }, "", "relative", "ftp://stt.test", "http://user:password@stt.test"),
		"BACKEND_TTS_URL":        text(func(c *Config, v string) { c.TTSURL = v }, "", "http://tts.test/#fragment"),
		"BACKEND_TTS_CFG_VALUE": {
			func(c *Config) { c.TTSCFGValue = -0.1 },
			func(c *Config) { c.TTSCFGValue = 5.1 },
			func(c *Config) { c.TTSCFGValue = math.NaN() },
			func(c *Config) { c.TTSCFGValue = math.Inf(1) },
		},
		"BACKEND_TTS_INFERENCE_TIMESTEPS": {
			func(c *Config) { c.TTSInferenceTimesteps = 0 },
			func(c *Config) { c.TTSInferenceTimesteps = 51 },
		},
		"BACKEND_VOICE_TIMEOUT": duration(func(c *Config, v time.Duration) { c.VoiceTimeout = v }, 0, -time.Second),
		"BACKEND_MAX_UPLOAD_BYTES": {
			func(c *Config) { c.MaxUploadBytes = 0 },
			func(c *Config) { c.MaxUploadBytes = -1 },
			func(c *Config) { c.MaxUploadBytes = 25*1024*1024 + 1 },
		},
		"BACKEND_S3_ENDPOINT":          text(func(c *Config, v string) { c.S3Endpoint = v }, "relative", "ftp://s3.test", "http://user:pass@s3.test", "http://s3.test/?x=1", "http://s3.test/#fragment"),
		"BACKEND_S3_REGION":            text(func(c *Config, v string) { c.S3Region = v }, "", "   "),
		"BACKEND_S3_BUCKET":            text(func(c *Config, v string) { c.S3Bucket = v }, "", "   "),
		"BACKEND_S3_ACCESS_KEY":        text(func(c *Config, v string) { c.S3AccessKey = v }, "", "   "),
		"BACKEND_S3_SECRET_KEY":        text(func(c *Config, v string) { c.S3SecretKey = v }, "", "   "),
		"BACKEND_S3_TIMEOUT":           duration(func(c *Config, v time.Duration) { c.S3Timeout = v }, 0, -time.Second),
		"BACKEND_LLM_GATEWAY_ADDR":     text(func(c *Config, v string) { c.LLMGatewayAddress = v }, "", "gateway", ":8011", "http://gateway:8011", "gateway:abc", "gateway:0", "gateway:-1", "gateway:65536", "gateway:70000"),
		"BACKEND_LLM_TIMEOUT":          duration(func(c *Config, v time.Duration) { c.LLMTimeout = v }, 0, -time.Second),
		"BACKEND_GEEK_QUESTION_LIMIT":  {func(c *Config) { c.GeekMaxQuestions = -1 }},
		"BACKEND_COURSE_GRAPH_TIMEOUT": duration(func(c *Config, v time.Duration) { c.CourseGraphTimeout = v }, -time.Second),
	}
	for key, mutations := range cases {
		for _, mutate := range mutations {
			c := testConfig()
			mutate(&c)
			if err := c.validate(); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("%s: got %v, want its validation error", key, err)
			}
		}
	}
}

func TestGatewayAddressAcceptsBoundaryPorts(t *testing.T) {
	for _, address := range []string{"llm-gateway:1", "llm-gateway:65535", "10.0.0.5:8011"} {
		c := testConfig()
		c.LLMGatewayAddress = address
		if err := c.validate(); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
}
