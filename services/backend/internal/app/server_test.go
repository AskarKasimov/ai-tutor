package app

import (
	"testing"
	"time"
)

func TestHTTPWriteTimeoutAllowsTranscriptionAndOCRToFinish(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		voice, s3, ocr, want time.Duration
	}{
		{"transcription", 120 * time.Second, 0, 60 * time.Second, 150 * time.Second},
		{"transcription with storage", 120 * time.Second, 30 * time.Second, 60 * time.Second, 300 * time.Second},
		{"ocr", time.Second, time.Second, 180 * time.Second, 210 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{VoiceTimeout: tc.voice, S3Timeout: tc.s3, CourseGraphTimeout: tc.ocr}
			if got := cfg.HTTPWriteTimeout(); got != tc.want {
				t.Fatalf("write deadline %s, want %s", got, tc.want)
			}
		})
	}
}
