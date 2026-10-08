package app

import (
	"testing"
	"time"
)

func TestHTTPWriteTimeoutAllowsDiagnosticAndGenerationToFinish(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		voice, assessment, generation, s3, want time.Duration
	}{
		{"generation", time.Second, 2 * time.Second, 90 * time.Second, 0, 120 * time.Second},
		{"assessment", time.Second, 180 * time.Second, 90 * time.Second, 0, 211 * time.Second},
		{"diagnostic", 120 * time.Second, 90 * time.Second, 45 * time.Second, 0, 240 * time.Second},
		{"audio repair", 120 * time.Second, 90 * time.Second, 45 * time.Second, 30 * time.Second, 300 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{VoiceTimeout: tc.voice, AssessmentTimeout: tc.assessment, TaskgenTimeout: tc.generation, S3Timeout: tc.s3}
			if got := cfg.HTTPWriteTimeout(); got != tc.want {
				t.Fatalf("write deadline %s, want %s", got, tc.want)
			}
		})
	}
}
