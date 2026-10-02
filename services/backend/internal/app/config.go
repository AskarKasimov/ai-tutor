package app

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddress     string
	DatabaseURL       string
	STTURL            string
	TTSURL            string
	AssessmentBaseURL string
	AssessmentModel   string
	ProcessingTimeout time.Duration
	AssessmentTimeout time.Duration
	MaxUploadBytes    int64
}

func ConfigFromEnv() (Config, error) {
	var c Config
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
	}
	for _, setting := range settings {
		value, err := requiredEnv(setting.key)
		if err != nil {
			return c, err
		}
		*setting.dst = value
	}
	durations := []struct {
		key string
		dst *time.Duration
	}{
		{"BACKEND_VOICE_TIMEOUT", &c.ProcessingTimeout},
		{"BACKEND_ASSESSMENT_TIMEOUT", &c.AssessmentTimeout},
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
	return c, c.validate()
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
	} {
		u, err := url.Parse(setting.value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", setting.key)
		}
	}
	if strings.TrimSpace(c.AssessmentModel) == "" {
		return fmt.Errorf("BACKEND_ASSESSMENT_MODEL is required")
	}
	for _, setting := range []struct {
		key   string
		value time.Duration
	}{
		{"BACKEND_VOICE_TIMEOUT", c.ProcessingTimeout},
		{"BACKEND_ASSESSMENT_TIMEOUT", c.AssessmentTimeout},
	} {
		if setting.value <= 0 {
			return fmt.Errorf("%s must be positive", setting.key)
		}
	}
	if c.MaxUploadBytes < 1 || c.MaxUploadBytes > 25*1024*1024 {
		return fmt.Errorf("BACKEND_MAX_UPLOAD_BYTES must be positive and cannot exceed 25 MiB")
	}
	return nil
}
