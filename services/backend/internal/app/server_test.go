package app

import (
	"testing"
	"time"
)

func TestHTTPWriteTimeoutAllowsSlowestModelToFinish(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		voice, assessment, generation, want time.Duration
	}{
		{"generation", time.Second, 2 * time.Second, 90 * time.Second, 120 * time.Second},
		{"assessment", time.Second, 180 * time.Second, 90 * time.Second, 210 * time.Second},
		{"voice", 120 * time.Second, 90 * time.Second, 45 * time.Second, 150 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ProcessingTimeout: tc.voice, AssessmentTimeout: tc.assessment, TaskgenTimeout: tc.generation}
			if got := cfg.HTTPWriteTimeout(); got != tc.want {
				t.Fatalf("write deadline %s, want %s", got, tc.want)
			}
		})
	}
}
