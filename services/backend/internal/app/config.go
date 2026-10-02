package app

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddress        string
	DatabaseURL          string
	STTURL               string
	TTSURL               string
	AssessmentBaseURL    string
	AssessmentModel      string
	PasswordCheckURL     string
	ProcessingTimeout    time.Duration
	AssessmentTimeout    time.Duration
	PasswordCheckTimeout time.Duration
	AuthRateLimit        int
	LoginEmailRateLimit  int
	MaxUploadBytes       int64
}

func DefaultConfig() Config {
	return Config{
		ListenAddress: ":8002",
		STTURL:        "http://localhost:8000/transcribe/longform", TTSURL: "http://localhost:8001/synthesize",
		AssessmentBaseURL: "http://10.100.10.105:30245/v1", AssessmentModel: "gpt-oss-120b",
		PasswordCheckURL:  "https://api.pwnedpasswords.com/range/",
		ProcessingTimeout: 120 * time.Second, AssessmentTimeout: 90 * time.Second, PasswordCheckTimeout: 5 * time.Second,
		AuthRateLimit: 30, LoginEmailRateLimit: 10, MaxUploadBytes: 25 * 1024 * 1024,
	}
}

func ConfigFromEnv() (Config, error) {
	c := DefaultConfig()
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	for key, dst := range map[string]*string{"LISTEN_ADDRESS": &c.ListenAddress, "STT_URL": &c.STTURL, "TTS_URL": &c.TTSURL, "ASSESSMENT_BASE_URL": &c.AssessmentBaseURL, "ASSESSMENT_MODEL": &c.AssessmentModel, "PASSWORD_CHECK_URL": &c.PasswordCheckURL} {
		if value := os.Getenv(key); value != "" {
			*dst = value
		}
	}
	if value := os.Getenv("PROCESSING_TIMEOUT"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return c, fmt.Errorf("PROCESSING_TIMEOUT: %w", err)
		}
		c.ProcessingTimeout = d
	}
	if value := os.Getenv("ASSESSMENT_TIMEOUT"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return c, fmt.Errorf("ASSESSMENT_TIMEOUT: %w", err)
		}
		c.AssessmentTimeout = d
	}
	if value := os.Getenv("AUTH_RATE_LIMIT"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return c, fmt.Errorf("AUTH_RATE_LIMIT must be an integer")
		}
		c.AuthRateLimit = n
	}
	return c, c.validate()
}

func (c Config) validate() error {
	for name, value := range map[string]string{"STT_URL": c.STTURL, "TTS_URL": c.TTSURL, "ASSESSMENT_BASE_URL": c.AssessmentBaseURL, "PASSWORD_CHECK_URL": c.PasswordCheckURL} {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", name)
		}
	}
	if c.ProcessingTimeout <= 0 || c.AssessmentTimeout <= 0 || c.PasswordCheckTimeout <= 0 || c.AssessmentModel == "" || c.AuthRateLimit < 1 || c.LoginEmailRateLimit < 1 || c.MaxUploadBytes < 1 || c.MaxUploadBytes > 25*1024*1024 {
		return fmt.Errorf("timeouts, rate limits and upload limit must be positive; uploads cannot exceed 25 MiB")
	}
	return nil
}
