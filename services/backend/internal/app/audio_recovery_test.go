package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAudioRecoveryRetriesUntilSuccessful(t *testing.T) {
	calls := 0
	completed := false
	err := recoverPublishedAudio(context.Background(), func(context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("temporary database error")
		}
		return nil
	}, func() { completed = true }, time.Millisecond, zap.NewNop())
	if err != nil || calls != 2 || !completed {
		t.Fatalf("err=%v calls=%d completed=%v", err, calls, completed)
	}
}

func TestAudioRecoveryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	completed := false
	err := recoverPublishedAudio(ctx, func(context.Context) error { cancel(); return errors.New("unavailable") }, func() { completed = true }, time.Hour, zap.NewNop())
	if err != nil || completed {
		t.Fatalf("err=%v completed=%v", err, completed)
	}
}
