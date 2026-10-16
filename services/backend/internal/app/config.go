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
	LLMBaseURLs    []string
	LLMModel       string
	LLMAPIKey      string
	LLMGRPCAddress string

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
	TutorBaseURL          string
	TutorModel            string
	TutorTimeout          time.Duration
	TutorEnabled          bool
	TutorTaskMaxChars     int
	TutorHistoryMaxChars  int
	TutorSummaryMaxChars  int
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
	CourseGraphURL        string
	CourseGraphTimeout    time.Duration
	OCRURL                string
	OCRTimeout            time.Duration
	TranslatorBaseURL     string
	TranslatorModel       string
	TranslatorAPIKey      string
	TranslatorTimeout     time.Duration
	TranslatorWorkers     int
}

// HTTPWriteTimeout allows sequential STT and grading, or task generation, to
// finish with time for request handling, persistence and writing the response.
func (c Config) HTTPWriteTimeout() time.Duration {
	return max(c.VoiceTimeout+c.AssessmentTimeout+c.TutorTimeout, c.VoiceTimeout+3*c.TutorTimeout, c.TaskgenTimeout, c.VoiceTimeout+5*c.S3Timeout, c.CourseGraphTimeout+c.TranslatorTimeout+5*c.S3Timeout) + 30*time.Second
}

func ConfigFromEnv() (Config, error) {
	var c Config
	var err error
	c.TutorEnabled, err = envBool("BACKEND_TUTOR_ENABLED", true)
	if err != nil {
		return c, err
	}
	settings := []struct {
		key string
		dst *string
	}{
		{"BACKEND_LISTEN_ADDRESS", &c.ListenAddress},
		{"BACKEND_DATABASE_URL", &c.DatabaseURL},
		{"BACKEND_STT_URL", &c.STTURL},
		{"BACKEND_TTS_URL", &c.TTSURL},
		{"BACKEND_ASSESSMENT_BASE_URL", &c.AssessmentBaseURL},
		{"BACKEND_ASSESSMENT_MODEL", &c.AssessmentModel},
		{"BACKEND_TASKGEN_BASE_URL", &c.TaskgenBaseURL},
		{"BACKEND_TASKGEN_MODEL", &c.TaskgenModel},
	}
	for _, setting := range settings {
		value, err := requiredEnv(setting.key)
		if err != nil {
			return c, err
		}
		*setting.dst = value
	}
	c.TutorBaseURL = envOrDefault("BACKEND_TUTOR_BASE_URL", c.AssessmentBaseURL)
	c.TutorModel = envOrDefault("BACKEND_TUTOR_MODEL", c.AssessmentModel)
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

	durations := []struct {
		key string
		dst *time.Duration
	}{
		{"BACKEND_VOICE_TIMEOUT", &c.VoiceTimeout},
		{"BACKEND_ASSESSMENT_TIMEOUT", &c.AssessmentTimeout},
		{"BACKEND_TASKGEN_TIMEOUT", &c.TaskgenTimeout},
	}
	c.TutorTimeout, err = envDuration("BACKEND_TUTOR_TIMEOUT", 60*time.Second)
	if err != nil {
		return c, err
	}
	c.TutorTaskMaxChars, err = envInt("BACKEND_TUTOR_TASK_MAX_CHARS", 16000)
	if err != nil {
		return c, err
	}
	c.TutorHistoryMaxChars, err = envInt("BACKEND_TUTOR_HISTORY_MAX_CHARS", 16000)
	if err != nil {
		return c, err
	}
	c.TutorSummaryMaxChars, err = envInt("BACKEND_TUTOR_SUMMARY_MAX_CHARS", 4000)
	if err != nil {
		return c, err
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
	value, err = requiredEnv("BACKEND_MAX_UPLOAD_BYTES")
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
	c.CourseGraphURL = strings.TrimSpace(os.Getenv("BACKEND_COURSE_GRAPH_URL"))
	if c.CourseGraphURL == "" {
		c.CourseGraphURL = strings.TrimSpace(os.Getenv("BACKEND_OCR_URL"))
	}
	c.CourseGraphTimeout, err = envDuration("BACKEND_COURSE_GRAPH_TIMEOUT", 4200*time.Second)
	if err != nil {
		return c, err
	}
	if os.Getenv("BACKEND_COURSE_GRAPH_TIMEOUT") == "" && os.Getenv("BACKEND_OCR_TIMEOUT") != "" {
		c.CourseGraphTimeout, err = envDuration("BACKEND_OCR_TIMEOUT", 120*time.Second)
		if err != nil {
			return c, err
		}
	}
	c.LLMGRPCAddress = envOrDefault("BACKEND_LLM_GRPC_ADDRESS", ":8003")
	c.LLMModel = envOrDefault("BACKEND_LLM_MODEL", c.AssessmentModel)
	c.LLMAPIKey = strings.TrimSpace(os.Getenv("BACKEND_LLM_API_KEY"))
	for _, endpoint := range strings.Split(envOrDefault("BACKEND_LLM_BASE_URLS", c.AssessmentBaseURL), ",") {
		c.LLMBaseURLs = append(c.LLMBaseURLs, strings.TrimSpace(endpoint))
	}
	c.OCRURL = c.CourseGraphURL
	c.OCRTimeout = c.CourseGraphTimeout
	c.TranslatorBaseURL = envOrDefault("BACKEND_TRANSLATOR_BASE_URL", envOrDefault("BACKEND_TUTOR_BASE_URL", c.AssessmentBaseURL))
	c.TranslatorModel = envOrDefault("BACKEND_TRANSLATOR_MODEL", envOrDefault("BACKEND_TUTOR_MODEL", c.AssessmentModel))
	c.TranslatorAPIKey = strings.TrimSpace(os.Getenv("BACKEND_TRANSLATOR_API_KEY"))
	if c.TranslatorAPIKey == "" {
		c.TranslatorAPIKey = c.LLMAPIKey
	}
	c.TranslatorTimeout, err = envDuration("BACKEND_TRANSLATOR_TIMEOUT", 120*time.Second)
	if err != nil {
		return c, err
	}
	c.TranslatorWorkers, err = envInt("BACKEND_TRANSLATOR_WORKERS", 4)
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

	_, port, err := net.SplitHostPort(c.ListenAddress)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("BACKEND_LISTEN_ADDRESS must be a host:port address with a port from 1 to 65535")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("BACKEND_DATABASE_URL is required")
	}
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
	if c.TutorEnabled {
		u, err := url.Parse(c.TutorBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("BACKEND_TUTOR_BASE_URL must be an absolute HTTP(S) URL without credentials")
		}
		if strings.TrimSpace(c.TutorModel) == "" {
			return fmt.Errorf("BACKEND_TUTOR_MODEL is required when tutor is enabled")
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
	if c.TutorTimeout <= 0 {
		return fmt.Errorf("BACKEND_TUTOR_TIMEOUT must be positive")
	}
	if c.TutorTaskMaxChars < 1 || c.TutorHistoryMaxChars < 1 || c.TutorSummaryMaxChars < 1 {
		return fmt.Errorf("BACKEND_TUTOR_*_MAX_CHARS values are invalid")
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
	if len(c.LLMBaseURLs) > 4 {
		return fmt.Errorf("BACKEND_LLM_BASE_URLS supports at most four endpoints")
	}
	for _, endpoint := range c.LLMBaseURLs {
		u, err := url.Parse(endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("BACKEND_LLM_BASE_URLS must contain absolute HTTP(S) URLs")
		}
	}
	if c.LLMGRPCAddress != "" {
		if _, _, err := net.SplitHostPort(c.LLMGRPCAddress); err != nil {
			return fmt.Errorf("BACKEND_LLM_GRPC_ADDRESS must be host:port")
		}
	}

	return nil
}
