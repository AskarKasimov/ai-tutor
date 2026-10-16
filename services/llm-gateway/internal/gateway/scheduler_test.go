package gateway

import (
	"testing"
	"time"
)

func schedulerFixture(instances []InstanceConfig, pool []string) *Service {
	cfg := Config{
		Pools:           map[string][]string{"low": pool, "medium": pool, "high": pool},
		Instances:       instances,
		UpstreamTimeout: time.Second,
	}
	return &Service{
		cfg:             cfg,
		jobs:            make(map[string]*durableJob),
		inFlight:        make(map[string]int),
		caller:          NewCaller(cfg),
		brokerAvailable: true,
		roundRobin:      make(map[string]int),
	}
}

func schedulerInstance(id string) InstanceConfig {
	return InstanceConfig{ID: id, Model: id, MaxInFlight: 1, ContextWindow: 8192}
}

func schedulerJob(id string, offset int64, urgent bool) *durableJob {
	return &durableJob{
		ID:     id,
		Offset: offset,
		Status: "queued",
		Request: Request{
			Messages:   []Message{{Role: "user", Content: "question"}},
			Complexity: "low",
			Urgent:     &urgent,
		},
	}
}

func TestNextSchedulesUrgentBeforeOlderNormalRequest(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.jobs["normal"] = schedulerJob("normal", 1, false)
	s.jobs["urgent"] = schedulerJob("urgent", 2, true)

	job, _, ok := s.next()
	if !ok {
		t.Fatal("next() returned no job")
	}
	if job.ID != "urgent" {
		t.Fatalf("scheduled job = %q, want urgent", job.ID)
	}
}

func TestNextPreservesFIFOWithinUrgentRequests(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a"), schedulerInstance("model-b")}, []string{"model-a", "model-b"})
	s.jobs["urgent-first"] = schedulerJob("urgent-first", 2, true)
	s.jobs["urgent-second"] = schedulerJob("urgent-second", 3, true)

	job, _, ok := s.next()
	if !ok || job.ID != "urgent-first" {
		t.Fatalf("next() = (%v, %v), want urgent-first", job, ok)
	}
}

func TestNextDoesNotScheduleWhenBrokerUnavailableOrSlotsAreFull(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Service)
	}{
		{name: "broker unavailable", setup: func(s *Service) { s.brokerAvailable = false }},
		{name: "instance slot full", setup: func(s *Service) { s.inFlight["model-a"] = 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
			s.jobs["queued"] = schedulerJob("queued", 1, false)
			tt.setup(s)
			if job, _, ok := s.next(); ok {
				t.Fatalf("next() = %q, want no schedulable job", job.ID)
			}
		})
	}
}

func TestNextRoundRobinsEqualLoadInstances(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a"), schedulerInstance("model-b")}, []string{"model-a", "model-b"})
	s.jobs["first"] = schedulerJob("first", 1, false)
	s.jobs["second"] = schedulerJob("second", 2, false)

	first, _, ok := s.next()
	if !ok || first.Instance != "model-a" {
		t.Fatalf("first selection = (%v, %v), want model-a", first, ok)
	}
	s.jobs[first.ID].Status = "running"
	s.inFlight[first.Instance]-- // The first request has completed; both instances are idle again.

	second, _, ok := s.next()
	if !ok || second.Instance != "model-b" {
		t.Fatalf("second selection = (%v, %v), want model-b", second, ok)
	}
}

func TestRestoreKeepsUncertainSlotPastResultRetention(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Retention = 24 * time.Hour
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)
	s.install(&durableJob{ID: "uncertain", Status: "failed", Instance: "model-a", SlotHeld: true, SubmittedAt: time.Now().Add(-48 * time.Hour), FinishedAt: time.Now().Add(-24 * time.Hour), Error: &storedError{Code: "upstream_timeout"}})
	if s.inFlight["model-a"] != 1 {
		t.Fatalf("restored in-flight count = %d, want uncertain slot retained", s.inFlight["model-a"])
	}
}

func TestRestoreRetainsResultFor24HoursAfterCompletion(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Retention = 24 * time.Hour
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)
	job := &durableJob{ID: "recent-result", Status: "succeeded", SubmittedAt: time.Now().Add(-48 * time.Hour), FinishedAt: time.Now().Add(-23 * time.Hour), Response: []byte(`{"id":"saved"}`)}
	s.install(job)
	if s.jobs[job.ID] == nil {
		t.Fatal("result was discarded before 24 hours elapsed after completion")
	}
}

func TestExpiredTerminalReplacesRestoredRunningJob(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Retention = 24 * time.Hour
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)
	now := time.Now()
	running := &durableJob{ID: "expired-terminal", ClientID: "client", IdempotencyKey: "key", Status: "running", Instance: "model-a", SlotHeld: true, SubmittedAt: now.Add(-48 * time.Hour), StartedAt: now.Add(-47 * time.Hour)}
	s.install(running)
	if s.inFlight["model-a"] != 1 {
		t.Fatalf("restored running slot count = %d, want 1", s.inFlight["model-a"])
	}

	terminal := &durableJob{ID: running.ID, ClientID: running.ClientID, IdempotencyKey: running.IdempotencyKey, Status: "succeeded", SubmittedAt: now.Add(-48 * time.Hour), FinishedAt: now.Add(-25 * time.Hour)}
	s.install(terminal)
	if _, ok := s.jobs[running.ID]; ok {
		t.Fatal("expired terminal left the earlier running job installed")
	}
	if s.inFlight["model-a"] != 0 {
		t.Fatalf("in-flight slot count = %d, want 0", s.inFlight["model-a"])
	}
	if len(s.stateIDs) != 0 || len(s.keys) != 0 {
		t.Fatalf("stale indexes remain: stateIDs=%v keys=%v", s.stateIDs, s.keys)
	}
}

func TestExpiredJobClearsCancellationForReusedKey(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Retention = time.Hour
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)
	s.cancelled = make(map[string]bool)
	now := time.Now()
	job := &durableJob{
		ID: "reused-key", ClientID: "internal", IdempotencyKey: "same-key",
		Status: "failed", Error: &storedError{Status: 409, Code: "request_canceled"},
		SubmittedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-time.Hour),
	}
	s.jobs[job.ID] = job
	s.keys[identity(job.ClientID, job.IdempotencyKey)] = job.ID
	s.stateIDs[string(stateKey(job))] = job.ID
	s.cancelled[job.ID] = true

	s.install(&durableJob{
		ID: job.ID, ClientID: job.ClientID, IdempotencyKey: job.IdempotencyKey,
		Status: "failed", Error: &storedError{Status: 409, Code: "request_canceled"},
		SubmittedAt: job.SubmittedAt, FinishedAt: job.FinishedAt,
	})

	if s.isCanceled(job.ID) {
		t.Fatal("expired cancellation flag still applies to the reused idempotency key")
	}
	if len(s.jobs) != 0 || len(s.keys) != 0 || len(s.stateIDs) != 0 {
		t.Fatalf("expired job indexes remain: jobs=%v keys=%v stateIDs=%v", s.jobs, s.keys, s.stateIDs)
	}
}
