package app

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIMode               string
	ListenAddress         string
	DatabaseURL           string
	STTURL                string
	TTSURL                string
	TTSSeed               int64
	TTSCFGValue           float64
	TTSInferenceTimesteps int
	AssessmentBaseURL     string
	AssessmentModel       string
	TaskgenBaseURL        string
	TaskgenModel          string
	TaskgenTimeout        time.Duration
	VoiceTimeout          time.Duration
	AssessmentTimeout     time.Duration
	MaxUploadBytes        int64
	S3Endpoint            string
	S3Region              string
	S3Bucket              string
	S3AccessKey           string
	S3SecretKey           string
	S3PathStyle           bool
	S3Timeout             time.Duration
	S3CreateBucket        bool
	AudioWorkers          int
	AudioPollInterval     time.Duration
}

// HTTPWriteTimeout allows sequential STT and grading, or task generation, to
// finish with time for request handling, persistence and writing the response.
func (c Config) HTTPWriteTimeout() time.Duration {
	return max(c.VoiceTimeout+c.AssessmentTimeout, c.TaskgenTimeout) + 30*time.Second
}

func ConfigFromEnv() (Config, error) {
	var c Config
	mode, err := requiredEnv("BACKEND_API_MODE")
	if err != nil {
		return c, err
	}
	c.APIMode = mode
	if err := validateAPIMode(mode); err != nil {
		return c, err
	}
	type stringSetting struct {
		key string
		dst *string
	}
	settings := []stringSetting{
		{"BACKEND_LISTEN_ADDRESS", &c.ListenAddress},
		{"BACKEND_DATABASE_URL", &c.DatabaseURL},
	}
	if c.APIMode == "real" {
		settings = append(settings,
			stringSetting{"BACKEND_STT_URL", &c.STTURL},
			stringSetting{"BACKEND_TTS_URL", &c.TTSURL},
			stringSetting{"BACKEND_ASSESSMENT_BASE_URL", &c.AssessmentBaseURL},
			stringSetting{"BACKEND_ASSESSMENT_MODEL", &c.AssessmentModel},
			stringSetting{"BACKEND_TASKGEN_BASE_URL", &c.TaskgenBaseURL},
			stringSetting{"BACKEND_TASKGEN_MODEL", &c.TaskgenModel},
		)
	}
	for _, setting := range settings {
		value, err := requiredEnv(setting.key)
		if err != nil {
			return c, err
		}
		*setting.dst = value
	}
	if c.APIMode == "real" {
		value, err := requiredEnv("BACKEND_TTS_SEED")
		if err != nil {
			return c, err
		}
		c.TTSSeed, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return c, fmt.Errorf("BACKEND_TTS_SEED: %w", err)
		}
		value, err = requiredEnv("BACKEND_TTS_CFG_VALUE")
		if err != nil {
			return c, err
		}
		c.TTSCFGValue, err = strconv.ParseFloat(value, 64)
		if err != nil {
			return c, fmt.Errorf("BACKEND_TTS_CFG_VALUE: %w", err)
		}
		value, err = requiredEnv("BACKEND_TTS_INFERENCE_TIMESTEPS")
		if err != nil {
			return c, err
		}
		c.TTSInferenceTimesteps, err = strconv.Atoi(value)
		if err != nil {
			return c, fmt.Errorf("BACKEND_TTS_INFERENCE_TIMESTEPS: %w", err)
		}
	}

	durations := []struct {
		key string
		dst *time.Duration
	}{
		{"BACKEND_VOICE_TIMEOUT", &c.VoiceTimeout},
		{"BACKEND_ASSESSMENT_TIMEOUT", &c.AssessmentTimeout},
	}
	if c.APIMode == "real" {
		durations = append(durations, struct {
			key string
			dst *time.Duration
		}{"BACKEND_TASKGEN_TIMEOUT", &c.TaskgenTimeout})
	}
	for _, setting := range durations {
		value, err := requiredEnv(setting.key)
		if err != nil {
			return c, err
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return c, fmt.Errorf("%s: %w", setting.key, err)
		}
		*setting.dst = duration
	}
	value, err := requiredEnv("BACKEND_MAX_UPLOAD_BYTES")
	if err != nil {
		return c, err
	}
	c.MaxUploadBytes, err = strconv.ParseInt(value, 10, 64)
	if err != nil {
		return c, fmt.Errorf("BACKEND_MAX_UPLOAD_BYTES: %w", err)
	}
	c.S3Endpoint = strings.TrimSpace(os.Getenv("BACKEND_S3_ENDPOINT"))
	c.S3Region = envOrDefault("BACKEND_S3_REGION", "us-east-1")
	c.S3Bucket, err = requiredEnv("BACKEND_S3_BUCKET")
	if err != nil {
		return c, err
	}
	c.S3AccessKey, err = requiredEnv("BACKEND_S3_ACCESS_KEY")
	if err != nil {
		return c, err
	}
	c.S3SecretKey, err = requiredEnv("BACKEND_S3_SECRET_KEY")
	if err != nil {
		return c, err
	}
	c.S3PathStyle, err = envBool("BACKEND_S3_PATH_STYLE", true)
	if err != nil {
		return c, err
	}
	c.S3Timeout, err = envDuration("BACKEND_S3_TIMEOUT", 30*time.Second)
	if err != nil {
		return c, err
	}
	c.S3CreateBucket, err = envBool("BACKEND_S3_CREATE_BUCKET", false)
	if err != nil {
		return c, err
	}
	c.AudioWorkers, err = envInt("BACKEND_AUDIO_WORKERS", 1)
	if err != nil {
		return c, err
	}
	c.AudioPollInterval, err = envDuration("BACKEND_AUDIO_POLL_INTERVAL", time.Second)
	if err != nil {
		return c, err
	}
	return c, c.validate()
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := envOrDefault(key, fallback.String())
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return duration, nil
}

func envInt(key string, fallback int) (int, error) {
	value := envOrDefault(key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func envBool(key string, fallback bool) (bool, error) {
	value := envOrDefault(key, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func (c Config) validate() error {
	if err := validateAPIMode(c.APIMode); err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(c.ListenAddress)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("BACKEND_LISTEN_ADDRESS must be a host:port address with a port from 1 to 65535")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("BACKEND_DATABASE_URL is required")
	}
	if c.APIMode == "real" {
		for _, setting := range []struct{ key, value string }{
			{"BACKEND_STT_URL", c.STTURL}, {"BACKEND_TTS_URL", c.TTSURL},
			{"BACKEND_ASSESSMENT_BASE_URL", c.AssessmentBaseURL},
			{"BACKEND_TASKGEN_BASE_URL", c.TaskgenBaseURL},
		} {
			u, err := url.Parse(setting.value)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
				return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", setting.key)
			}
		}
		if math.IsNaN(c.TTSCFGValue) || math.IsInf(c.TTSCFGValue, 0) || c.TTSCFGValue < 0 || c.TTSCFGValue > 5 {
			return fmt.Errorf("BACKEND_TTS_CFG_VALUE must be a finite number from 0 to 5")
		}
		if c.TTSInferenceTimesteps < 1 || c.TTSInferenceTimesteps > 50 {
			return fmt.Errorf("BACKEND_TTS_INFERENCE_TIMESTEPS must be from 1 to 50")
		}
		if strings.TrimSpace(c.TaskgenModel) == "" {
			return fmt.Errorf("BACKEND_TASKGEN_MODEL is required")
		}
		if c.TaskgenTimeout <= 0 {
			return fmt.Errorf("BACKEND_TASKGEN_TIMEOUT must be positive")
		}
		if strings.TrimSpace(c.AssessmentModel) == "" {
			return fmt.Errorf("BACKEND_ASSESSMENT_MODEL is required")
		}
	}
	for _, setting := range []struct {
		key   string
		value time.Duration
	}{
		{"BACKEND_VOICE_TIMEOUT", c.VoiceTimeout},
		{"BACKEND_ASSESSMENT_TIMEOUT", c.AssessmentTimeout},
	} {
		if setting.value <= 0 {
			return fmt.Errorf("%s must be positive", setting.key)
		}
	}
	if strings.TrimSpace(c.S3Bucket) == "" {
		return fmt.Errorf("BACKEND_S3_BUCKET is required")
	}
	if strings.TrimSpace(c.S3AccessKey) == "" {
		return fmt.Errorf("BACKEND_S3_ACCESS_KEY is required")
	}
	if strings.TrimSpace(c.S3SecretKey) == "" {
		return fmt.Errorf("BACKEND_S3_SECRET_KEY is required")
	}
	if strings.TrimSpace(c.S3Region) == "" {
		return fmt.Errorf("BACKEND_S3_REGION is required")
	}
	if c.S3Endpoint != "" {
		u, err := url.Parse(c.S3Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("BACKEND_S3_ENDPOINT must be an absolute HTTP(S) URL without credentials, query or fragment")
		}
	}
	if c.S3Timeout <= 0 {
		return fmt.Errorf("BACKEND_S3_TIMEOUT must be positive")
	}
	if c.AudioWorkers <= 0 {
		return fmt.Errorf("BACKEND_AUDIO_WORKERS must be positive")
	}
	if c.AudioPollInterval <= 0 {
		return fmt.Errorf("BACKEND_AUDIO_POLL_INTERVAL must be positive")
	}
	if c.MaxUploadBytes < 1 || c.MaxUploadBytes > 25*1024*1024 {
		return fmt.Errorf("BACKEND_MAX_UPLOAD_BYTES must be positive and cannot exceed 25 MiB")
	}
	return nil
}

func validateAPIMode(mode string) error {
	if mode != "mock" && mode != "real" {
		return fmt.Errorf("BACKEND_API_MODE must be mock or real")
	}
	return nil
}
