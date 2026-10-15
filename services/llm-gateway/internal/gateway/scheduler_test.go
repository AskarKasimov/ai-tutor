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
