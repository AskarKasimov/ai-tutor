package app

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
)

type Config struct {
	LogLevel             zapcore.Level
	ListenAddress        string
	DatabaseURL          string
	VoiceGatewayAddress  string
	VoiceProfile         string
	VoiceTimeout         time.Duration
	MaxUploadBytes       int64
	S3Endpoint           string
	S3Region             string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	S3PathStyle          bool
	S3Timeout            time.Duration
	S3CreateBucket       bool
	AudioWorkers         int
	AudioPollInterval    time.Duration
	LearningTutorEnabled bool
	TutorTaskMaxChars    int
	TutorHistoryMaxChars int
	TutorSummaryMaxChars int
	CourseGraphURL       string
	CourseGraphTimeout   time.Duration
	OCRURL               string
	OCRTimeout           time.Duration
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
	if host, port, err := net.SplitHostPort(c.VoiceGatewayAddress); err != nil || host == "" || !validPort(port) {
		return fmt.Errorf("voice.address must be a host:port gRPC address")
	}
	if strings.TrimSpace(c.VoiceProfile) == "" {
		return fmt.Errorf("voice.profile is required")
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
