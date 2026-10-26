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
	ListenAddress         string
	DatabaseURL           string
	STTURL                string
	TTSURL                string
	TTSSeed               int64
	TTSCFGValue           float64
	TTSInferenceTimesteps int
	VoiceTimeout          time.Duration
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
	LearningTutorEnabled  bool
	TutorTaskMaxChars     int
	TutorHistoryMaxChars  int
	TutorSummaryMaxChars  int
	CourseGraphURL        string
	CourseGraphTimeout    time.Duration
	OCRURL                string
	OCRTimeout            time.Duration
	// LLMGatewayAddress is the gRPC address (host:port) of the LLM gateway.
	LLMGatewayAddress string
	LLMTimeout        time.Duration
	// GeekMaxQuestions stops a diagnostic after that many questions and
	// followups; 0 is no limit.
	GeekMaxQuestions int
}

// HTTPWriteTimeout allows STT with S3 writes, OCR of an uploaded material, a
// Geek answer (evaluation and the next question, each retried once), a long
// Geek turn, or learning STT followed by up to four sequential tutor calls to
// finish, with time for request handling,
// persistence and writing the response.
func (c Config) HTTPWriteTimeout() time.Duration {
	return max(c.VoiceTimeout+5*c.S3Timeout, c.CourseGraphTimeout, 4*c.LLMTimeout, c.VoiceTimeout+4*c.LLMTimeout, 4*c.VoiceTimeout+2*c.S3Timeout) + 30*time.Second
}

func ConfigFromEnv() (Config, error) {
	var c Config
	var err error
	settings := []struct {
		key string
		dst *string
	}{
		{"BACKEND_LISTEN_ADDRESS", &c.ListenAddress},
		{"BACKEND_DATABASE_URL", &c.DatabaseURL},
		{"BACKEND_STT_URL", &c.STTURL},
		{"BACKEND_TTS_URL", &c.TTSURL},
	}
	for _, setting := range settings {
		value, err := requiredEnv(setting.key)
		if err != nil {
			return c, err
		}
		*setting.dst = value
	}
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
	// VoxCPM gets the steps in every request; 20 sounds like 50 at half the
	// synthesis time (measured 17.10.2026), so an empty value means 20.
	c.TTSInferenceTimesteps, err = strconv.Atoi(envOrDefault("BACKEND_TTS_INFERENCE_TIMESTEPS", strconv.Itoa(DefaultTTSInferenceTimesteps)))
	if err != nil {
		return c, fmt.Errorf("BACKEND_TTS_INFERENCE_TIMESTEPS: %w", err)
	}

	value, err = requiredEnv("BACKEND_VOICE_TIMEOUT")
	if err != nil {
		return c, err
	}
	c.VoiceTimeout, err = time.ParseDuration(value)
	if err != nil {
		return c, fmt.Errorf("BACKEND_VOICE_TIMEOUT: %w", err)
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
	c.LearningTutorEnabled, err = envBool("BACKEND_TUTOR_ENABLED", true)
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
	c.CourseGraphURL = strings.TrimSpace(os.Getenv("BACKEND_COURSE_GRAPH_URL"))
	if c.CourseGraphURL == "" {
		c.CourseGraphURL = strings.TrimSpace(os.Getenv("BACKEND_OCR_URL"))
	}
	// 0 means no overall HTTP timeout: OCR of a long PDF and topic extraction
	// are bounded by the worker lease and context instead.
	c.CourseGraphTimeout, err = envDuration("BACKEND_COURSE_GRAPH_TIMEOUT", 0)
	if err != nil {
		return c, err
	}
	if os.Getenv("BACKEND_COURSE_GRAPH_TIMEOUT") == "" && os.Getenv("BACKEND_OCR_TIMEOUT") != "" {
		c.CourseGraphTimeout, err = envDuration("BACKEND_OCR_TIMEOUT", 0)
		if err != nil {
			return c, err
		}
	}
	c.OCRURL = c.CourseGraphURL
	c.OCRTimeout = c.CourseGraphTimeout
	c.LLMGatewayAddress, err = requiredEnv("BACKEND_LLM_GATEWAY_ADDR")
	if err != nil {
		return c, err
	}
	c.LLMTimeout, err = envDuration("BACKEND_LLM_TIMEOUT", 120*time.Second)
	if err != nil {
		return c, err
	}
	value = envOrDefault("BACKEND_GEEK_QUESTION_LIMIT", strconv.Itoa(DefaultGeekQuestionLimit))
	if c.GeekMaxQuestions, err = strconv.Atoi(value); err != nil {
		return c, fmt.Errorf("BACKEND_GEEK_QUESTION_LIMIT: %w", err)
	}
	return c, c.validate()
}

// DefaultTTSInferenceTimesteps is sent to VoxCPM when the env leaves it empty.
const DefaultTTSInferenceTimesteps = 20

// DefaultGeekQuestionLimit stops a diagnostic when the env leaves the limit
// empty (decision of 18.10.2026).
const DefaultGeekQuestionLimit = 8

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

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
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
	if c.VoiceTimeout <= 0 {
		return fmt.Errorf("BACKEND_VOICE_TIMEOUT must be positive")
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
	if c.AudioWorkers < 1 || c.AudioPollInterval <= 0 {
		return fmt.Errorf("BACKEND_AUDIO_WORKERS and BACKEND_AUDIO_POLL_INTERVAL must be positive")
	}
	if c.TutorTaskMaxChars < 1 || c.TutorHistoryMaxChars < 1 || c.TutorSummaryMaxChars < 1 {
		return fmt.Errorf("learning tutor context limits must be positive")
	}
	if host, port, err := net.SplitHostPort(c.LLMGatewayAddress); err != nil || host == "" || !validPort(port) {
		return fmt.Errorf("BACKEND_LLM_GATEWAY_ADDR must be a host:port gRPC address with a port from 1 to 65535")
	}
	if c.LLMTimeout <= 0 {
		return fmt.Errorf("BACKEND_LLM_TIMEOUT must be positive")
	}
	if c.CourseGraphTimeout < 0 {
		return fmt.Errorf("BACKEND_COURSE_GRAPH_TIMEOUT must not be negative")
	}
	if c.GeekMaxQuestions < 0 {
		return fmt.Errorf("BACKEND_GEEK_QUESTION_LIMIT must not be negative")
	}
	if c.MaxUploadBytes < 1 || c.MaxUploadBytes > 25*1024*1024 {
		return fmt.Errorf("BACKEND_MAX_UPLOAD_BYTES must be positive and cannot exceed 25 MiB")
	}
	return nil
}
