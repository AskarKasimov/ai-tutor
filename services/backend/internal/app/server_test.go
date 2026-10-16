package app

import (
	"testing"
	"time"
)

func TestHTTPWriteTimeoutAllowsSlowOperationsToFinish(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		voice, s3, ocr, llm, want time.Duration
	}{
		{"transcription with storage", 30 * time.Second, 30 * time.Second, 60 * time.Second, 0, 210 * time.Second},
		{"ocr", time.Second, time.Second, 180 * time.Second, 0, 210 * time.Second},
		{"geek answer", time.Second, time.Second, time.Second, 120 * time.Second, 510 * time.Second},
		{"long geek turn", 120 * time.Second, 10 * time.Second, time.Second, time.Second, 530 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{VoiceTimeout: tc.voice, S3Timeout: tc.s3, CourseGraphTimeout: tc.ocr, LLMTimeout: tc.llm}
			if got := cfg.HTTPWriteTimeout(); got != tc.want {
				t.Fatalf("write deadline %s, want %s", got, tc.want)
			}
		})
	}
}
