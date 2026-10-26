package postgres

import (
	"testing"
	"time"
)

func TestTrainingRepositoryWithLeaseRoundsUpToWholeSeconds(t *testing.T) {
	repository := New(nil).WithLease(5*time.Second + time.Nanosecond)
	if repository.leaseSeconds != 6 {
		t.Fatalf("lease seconds = %d, want 6", repository.leaseSeconds)
	}
}
