package gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// halfOpen leaves the breaker of model-a open with its pause over: the next
// reservation is the half-open probe.
func halfOpen(s *Service) {
	c := s.caller.circuit("model-a")
	c.mu.Lock()
	c.failures, c.openUntil, c.probing = 3, time.Now().Add(-time.Second), false
	c.mu.Unlock()
}

func probeGranted(s *Service) bool {
	ticket, ok := s.caller.circuit("model-a").reserve(time.Now())
	return ok && ticket.probe
}

// runDispatchUntilExpired runs the dispatcher until the job is no longer queued.
func runDispatchUntilExpired(t *testing.T, s *Service, id string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.dispatch(ctx, ctx) }()
	s.signal()
	deadline := time.Now().Add(3 * time.Second)
	for s.job(id).Status != "expired" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if got := s.job(id).Status; got != "expired" {
		t.Fatalf("job status = %q, want expired", got)
	}
}

func TestDispatchExpiryDuringRunningWriteReleasesTheProbe(t *testing.T) {
	for _, probe := range []bool{true, false} {
		name := "no probe"
		if probe {
			name = "probe"
		}
		t.Run(name, func(t *testing.T) {
			s, stub := repairFixture(t)
			var slow atomic.Bool
			s.writeStateFn = func(ctx context.Context, key, value []byte) error {
				if slow.CompareAndSwap(false, true) {
					time.Sleep(80 * time.Millisecond)
				}
				return stub.write(ctx, key, value)
			}
			if probe {
				halfOpen(s)
			}
			q := schedulerJob("j", 100, false)
			q.SubmittedAt, q.Deadline = time.Now(), time.Now().Add(40*time.Millisecond)
			s.mu.Lock()
			s.jobs["j"] = q
			s.mu.Unlock()

			runDispatchUntilExpired(t, s, "j")

			if got := s.slots(); got != 1 {
				t.Fatalf("in flight = %d, want 1 (the bystander only)", got)
			}
			if e := s.job("j").Error; e == nil || e.Status != 504 || e.Code != "request_deadline_exceeded" {
				t.Fatalf("stored error = %+v, want 504 request_deadline_exceeded", e)
			}
			if probe {
				if !probeGranted(s) {
					t.Fatal("the half-open probe stayed reserved by the expired job")
				}
			}
		})
	}
}

func TestExpireDispatchedReleasesProbeAndSlotOnce(t *testing.T) {
	for _, probe := range []bool{true, false} {
		s, _ := repairFixture(t)
		if probe {
			halfOpen(s)
		}
		j := runningJob(s, "j", time.Now().Add(-time.Second))
		if probe {
			if !probeGranted(s) {
				t.Fatal("setup: no probe")
			}
			j.ProbeReserved = true
		}
		s.expireDispatched(j)
		if got := s.slots(); got != 1 {
			t.Fatalf("probe=%v: in flight = %d, want 1", probe, got)
		}
		if j.ProbeReserved || j.SlotHeld {
			t.Fatalf("probe=%v: flags not cleared: %+v", probe, j)
		}
		if st := s.job("j"); st.Status != "expired" || st.SlotHeld {
			t.Fatalf("probe=%v: job = %+v", probe, st)
		}
		if probe && !probeGranted(s) {
			t.Fatal("the probe stayed reserved")
		}
	}
}

func TestExecuteExpiredReleasesProbeAndSlotOnce(t *testing.T) {
	for _, probe := range []bool{true, false} {
		s, _ := repairFixture(t)
		if probe {
			halfOpen(s)
			if !probeGranted(s) {
				t.Fatal("setup: no probe")
			}
		}
		j := runningJob(s, "j", time.Now().Add(-time.Second))
		j.ProbeReserved = probe
		s.execute(context.Background(), j, s.instance("model-a"))
		if got := s.slots(); got != 1 {
			t.Fatalf("probe=%v: in flight = %d, want 1", probe, got)
		}
		if st := s.job("j"); st.Status != "expired" {
			t.Fatalf("probe=%v: status = %q, want expired", probe, st.Status)
		}
		if probe && !probeGranted(s) {
			t.Fatal("the probe stayed reserved")
		}
	}
}
