package gateway

import (
	"context"
	"errors"
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

func TestRestoreReleasesSlotForInterruptedExecution(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Retention = 24 * time.Hour
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)

	now := time.Now()
	for _, job := range []*durableJob{
		{ID: "running-at-restart", Status: "running", Instance: "model-a", SlotHeld: true, SubmittedAt: now, StartedAt: now.Add(-2 * time.Second)},
		{ID: "interrupted-before-restart", Status: "failed", Instance: "model-a", SlotHeld: true, SubmittedAt: now, StartedAt: now.Add(-2 * time.Second), FinishedAt: now, Error: &storedError{Code: "execution_interrupted"}},
	} {
		s.install(job)
	}

	if got := s.inFlight["model-a"]; got != 0 {
		t.Fatalf("restored in-flight count = %d, want 0 after interrupted jobs release their slots", got)
	}
}

func TestRestoreKeepsInterruptedSlotUntilUpstreamTimeout(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Instances[0].Timeout = time.Second
	s.stateIDs = make(map[string]string)
	s.keys = make(map[string]string)
	now := time.Now()
	s.install(&durableJob{ID: "recent-interruption", Status: "running", Instance: "model-a", SlotHeld: true, SubmittedAt: now, StartedAt: now.Add(-500 * time.Millisecond)})

	if got := s.inFlight["model-a"]; got != 1 {
		t.Fatalf("restored in-flight count = %d, want 1 until upstream timeout", got)
	}
}

func TestUncertainSlotExpiresAtConfiguredUpstreamTimeout(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.cfg.Instances[0].Timeout = time.Second
	now := time.Now()
	job := &durableJob{ID: "uncertain", Status: "failed", Instance: "model-a", SlotHeld: true, StartedAt: now.Add(-time.Second)}
	if !s.uncertainSlotExpired(job, now) {
		t.Fatal("slot should expire at the configured upstream timeout")
	}
	job.StartedAt = now.Add(-time.Millisecond)
	if s.uncertainSlotExpired(job, now) {
		t.Fatal("slot expired before the configured upstream timeout")
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
	if s.inFlight["model-a"] != 0 {
		t.Fatalf("restored running slot count = %d, want 0 after interruption", s.inFlight["model-a"])
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

// tieredFixture has a low "slow" instance and a high "fast" one.
func tieredFixture() *Service {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("slow"), schedulerInstance("fast")}, nil)
	s.cfg.Pools = map[string][]string{"low": {"slow"}, "medium": {"fast"}, "high": {"fast"}}
	s.caller = NewCaller(s.cfg)
	return s
}

func TestNextPrefersTheFreeInstanceThatAnswersSooner(t *testing.T) {
	s := tieredFixture()
	s.expected = map[string]float64{"slow": 10, "fast": 2}
	s.jobs["low"] = schedulerJob("low", 1, false)
	job, _, ok := s.next()
	if !ok || job.Instance != "fast" {
		t.Fatalf("a low request goes to the faster free instance, got %v", job)
	}
}

func TestNextTriesAnInstanceWithoutSamplesFirst(t *testing.T) {
	s := tieredFixture()
	s.expected = map[string]float64{"fast": 1}
	s.jobs["low"] = schedulerJob("low", 1, false)
	if job, _, ok := s.next(); !ok || job.Instance != "slow" {
		t.Fatalf("an unmeasured instance is tried first, got %v", job)
	}
}

func TestNextUsesTheSlowerInstanceWhenTheFasterIsBusy(t *testing.T) {
	s := tieredFixture()
	s.expected = map[string]float64{"slow": 10, "fast": 2}
	s.inFlight["fast"] = 1
	s.jobs["low"] = schedulerJob("low", 1, false)
	if job, _, ok := s.next(); !ok || job.Instance != "slow" {
		t.Fatalf("load comes before speed, got %v", job)
	}
}

func TestNextKeepsTheLessCapableTierOnEqualEstimates(t *testing.T) {
	s := tieredFixture()
	s.jobs["low"] = schedulerJob("low", 1, false)
	if job, _, ok := s.next(); !ok || job.Instance != "slow" {
		t.Fatalf("with no estimates the less capable tier goes first, got %v", job)
	}
}

func TestObserveCallAveragesSuccessesAndTimeoutsOnly(t *testing.T) {
	s := tieredFixture()
	in := schedulerInstance("fast")
	job := schedulerJob("j", 1, false)
	s.observeCall(job, in, nil, 10*time.Second)
	if s.expected["fast"] != 10 {
		t.Fatalf("first sample = %v", s.expected["fast"])
	}
	s.observeCall(job, in, nil, 20*time.Second)
	if got := s.expected["fast"]; got < 12.99 || got > 13.01 {
		t.Fatalf("moving average = %v, want 13", got)
	}
	ignored := []*UpstreamFailure{
		{BeforeSend: true, Err: context.DeadlineExceeded},
		{Status: 429, Err: context.DeadlineExceeded},
		{Status: 500, Err: errors.New("upstream 500")},
	}
	for _, failure := range ignored {
		s.observeCall(job, in, failure, time.Millisecond)
	}
	if got := s.expected["fast"]; got < 12.99 || got > 13.01 {
		t.Fatalf("failures that say nothing about speed changed the estimate: %v", got)
	}
	s.observeCall(job, in, &UpstreamFailure{Err: context.DeadlineExceeded}, 120*time.Second)
	if got := s.expected["fast"]; got < 45.09 || got > 45.11 {
		t.Fatalf("a timeout counts as a slow call: %v, want 45.1", got)
	}
}

func TestAnInstanceThatFailedGoesAfterAHealthyOne(t *testing.T) {
	// The reviewed case: "fast" was never measured, so it has the first
	// place, and keeps answering 429 while "slow" is healthy and free.
	s := tieredFixture()
	s.expected = map[string]float64{"slow": 2}
	job := schedulerJob("low", 1, false)
	s.jobs["low"] = job
	if picked, _, ok := s.next(); !ok || picked.Instance != "fast" {
		t.Fatalf("the unmeasured instance is tried first, got %v", picked)
	}
	s.inFlight["fast"]--
	s.observeCall(job, schedulerInstance("fast"), &UpstreamFailure{Status: 429, Err: errors.New("rate limited")}, time.Millisecond)
	s.jobs["low"].Status = "queued"
	if picked, _, ok := s.next(); !ok || picked.Instance != "slow" {
		t.Fatalf("after a 429 the healthy instance goes first, got %v", picked)
	}
	// With the healthy one busy, the cooling one still serves: no stall.
	s.jobs["next"] = schedulerJob("next", 2, false)
	if picked, _, ok := s.next(); !ok || picked.Instance != "fast" {
		t.Fatalf("a cooling instance still takes work when it is the only free one, got %v", picked)
	}
}

func TestCooldownFollowsRetryAfterAndASuccessEndsIt(t *testing.T) {
	s := tieredFixture()
	in := schedulerInstance("fast")
	job := schedulerJob("j", 1, false)
	before := time.Now()
	s.observeCall(job, in, &UpstreamFailure{Status: 503, Err: errors.New("down"), RetryAfter: time.Minute}, time.Millisecond)
	if until := s.cooling["fast"]; until.Before(before.Add(59 * time.Second)) {
		t.Fatalf("cooldown until %v, want Retry-After", until)
	}
	s.observeCall(job, in, nil, time.Second)
	if _, cooling := s.cooling["fast"]; cooling {
		t.Fatal("a successful call ends the cooldown")
	}
	for _, failure := range []*UpstreamFailure{
		{Code: "request_canceled", Err: context.Canceled},
		{Code: "upstream_circuit_open", Err: errors.New("open"), BeforeSend: true},
		{Status: 400, Err: errors.New("bad request")},
	} {
		s.observeCall(job, in, failure, time.Millisecond)
		if _, cooling := s.cooling["fast"]; cooling {
			t.Fatalf("%+v must not put the instance on cooldown", failure)
		}
	}
	s.observeCall(job, in, &UpstreamFailure{Code: "upstream_transport_error", Err: context.DeadlineExceeded, BeforeSend: true}, time.Millisecond)
	if _, cooling := s.cooling["fast"]; !cooling || s.expected["fast"] != 1 {
		t.Fatalf("a connect timeout is a failure, not a fast call: cooling=%v expected=%v", s.cooling["fast"], s.expected["fast"])
	}
}
