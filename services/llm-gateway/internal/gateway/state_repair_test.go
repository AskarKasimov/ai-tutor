package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// stateStub stands in for the transactional write to the state topic.
type stateStub struct {
	mu       sync.Mutex
	fail     bool
	attempts int
	writes   []durableJob
}

func (st *stateStub) write(_ context.Context, _, value []byte) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.attempts++
	if st.fail {
		return errors.New("broker unavailable")
	}
	var j durableJob
	if err := json.Unmarshal(value, &j); err != nil {
		return err
	}
	st.writes = append(st.writes, j)
	return nil
}

func (st *stateStub) setFail(fail bool) { st.mu.Lock(); st.fail = fail; st.mu.Unlock() }
func (st *stateStub) attemptCount() int { st.mu.Lock(); defer st.mu.Unlock(); return st.attempts }
func (st *stateStub) written(id string) []durableJob {
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []durableJob
	for _, j := range st.writes {
		if j.ID == id {
			out = append(out, j)
		}
	}
	return out
}

// repairFixture is a service with two slots on model-a: one is held by the
// bystander job, so a slot released twice shows in the count.
func repairFixture(t *testing.T) (*Service, *stateStub) {
	t.Helper()
	cfg := Config{KafkaBrokers: []string{"127.0.0.1:1"}, MaxConnections: 1}
	s, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	instance := schedulerInstance("model-a")
	instance.MaxInFlight = 2
	s.cfg.Instances = []InstanceConfig{instance}
	s.cfg.Pools = map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}}
	s.cfg.RequestDeadline = time.Minute
	s.caller = NewCaller(s.cfg)
	stub := &stateStub{}
	s.writeStateFn = stub.write
	bystander := schedulerJob("bystander", 1, false)
	bystander.Status, bystander.Instance, bystander.SlotHeld = "running", "model-a", true
	bystander.Deadline = time.Now().Add(time.Hour)
	s.mu.Lock()
	s.jobs[bystander.ID] = bystander
	s.inFlight["model-a"] = 1
	s.ready, s.brokerAvailable = true, true
	s.mu.Unlock()
	return s, stub
}

// runningJob is a job dispatched to model-a that holds a slot.
func runningJob(s *Service, id string, deadline time.Time) *durableJob {
	j := schedulerJob(id, 10, false)
	j.Status, j.Instance, j.SlotHeld, j.Attempt = "running", "model-a", true, 1
	j.SubmittedAt, j.Deadline = time.Now(), deadline
	s.mu.Lock()
	s.jobs[id] = j
	s.inFlight["model-a"]++
	s.mu.Unlock()
	copy := *j
	return &copy
}

func (s *Service) slots() int { s.mu.Lock(); defer s.mu.Unlock(); return s.inFlight["model-a"] }
func (s *Service) job(id string) durableJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		return *j
	}
	return durableJob{}
}

// addQueued puts a job in the queue; with state writes working, next() hands it out.
func addQueued(s *Service, id string) {
	q := schedulerJob(id, 100, false)
	q.SubmittedAt, q.Deadline = time.Now(), time.Now().Add(time.Hour)
	s.mu.Lock()
	s.jobs[id] = q
	s.mu.Unlock()
}

type repairCase struct {
	name string
	// trigger runs the path whose state write fails and returns the number of
	// slots model-a holds, beyond the bystander's, once the repair is done.
	trigger func(t *testing.T, s *Service) (id string, slotsAfter int)
	status  string
	// afterRepair is what the interrupted caller does once its path continues.
	afterRepair func(s *Service)
}

func repairCases() []repairCase {
	return []repairCase{
		{name: "cancellation", status: "failed", trigger: func(t *testing.T, s *Service) (string, int) {
			runningJob(s, "j", time.Now().Add(time.Hour))
			s.mu.Lock()
			s.cancelled["j"] = true
			s.mu.Unlock()
			s.persistCancellation("j", "request_canceled")
			return "j", 1
		}, afterRepair: func(s *Service) {
			// execute sees the cancellation and releases the slot itself
			s.releaseSlot(&durableJob{ID: "j", Instance: "model-a"})
		}},
		{name: "reschedule", status: "queued", trigger: func(t *testing.T, s *Service) (string, int) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			t.Cleanup(server.Close)
			in := s.cfg.Instances[0]
			in.BaseURL = server.URL + "/v1"
			in.Timeout = 2 * time.Second
			j := runningJob(s, "j", time.Now().Add(time.Hour))
			s.execute(context.Background(), j, in)
			return "j", 0
		}},
		{name: "result", status: "succeeded", trigger: func(t *testing.T, s *Service) (string, int) {
			j := runningJob(s, "j", time.Now().Add(-time.Second))
			j.SlotHeld = false
			if err := s.persistResult(context.Background(), j, 200, []byte(`{"ok":true}`), "succeeded"); err == nil {
				t.Fatal("persistResult succeeded while the write fails")
			}
			// execute does not release the slot after a failed save
			return "j", 0
		}},
		{name: "finish", status: "expired", trigger: func(t *testing.T, s *Service) (string, int) {
			j := runningJob(s, "j", time.Now().Add(-time.Second))
			j.SlotHeld = false
			if s.finish(j, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "LLM request deadline exceeded.")}, "expired") {
				t.Fatal("finish succeeded while the write fails")
			}
			s.releaseSlot(j) // dispatch and execute release it unconditionally
			return "j", 0
		}},
		{name: "slot_release", status: "failed", trigger: func(t *testing.T, s *Service) (string, int) {
			s.cfg.Instances[0].ServerExecutionLimit = time.Second
			j := runningJob(s, "j", time.Now().Add(time.Hour))
			s.mu.Lock()
			held := s.jobs["j"]
			held.Status, held.FinishedAt = "failed", time.Now().Add(-time.Minute)
			held.Error = &storedError{Status: 502, Type: "server_error", Code: "upstream_response_error", Message: "failed"}
			s.mu.Unlock()
			_ = j
			s.releaseExpiredUncertainSlots(time.Now())
			return "j", 0
		}},
	}
}

func TestStateWriteFailureBlocksAssignmentsUntilRepaired(t *testing.T) {
	for _, tc := range repairCases() {
		t.Run(tc.name, func(t *testing.T) {
			s, stub := repairFixture(t)
			addQueued(s, "q")
			stub.setFail(true)
			id, slotsAfter := tc.trigger(t, s)

			if s.Ready() {
				t.Fatal("gateway stayed ready after a state write failed")
			}
			if j, _, ok := s.next(); ok {
				t.Fatalf("next() assigned %s while the state is uncertain", j.ID)
			}

			// The broker answers a ping but the write still fails: nothing changes.
			before := stub.attemptCount()
			s.readinessTick(context.Background(), nil)
			if stub.attemptCount() == before {
				t.Fatal("the reachable broker did not start a repair attempt")
			}
			if s.Ready() {
				t.Fatal("a successful ping made the gateway ready while the write still fails")
			}
			s.mu.Lock()
			uncertain := s.stateUncertain
			s.mu.Unlock()
			if !uncertain {
				t.Fatal("uncertainty was cleared although the state is still not written")
			}
			if j, _, ok := s.next(); ok {
				t.Fatalf("next() assigned %s after a ping only", j.ID)
			}

			stub.setFail(false)
			s.readinessTick(context.Background(), nil)
			if !s.Ready() {
				t.Fatal("gateway did not become ready after the state was written")
			}
			written := stub.written(id)
			if len(written) != 1 || written[0].Status != tc.status {
				t.Fatalf("written states of %s = %+v, want one with status %s", id, written, tc.status)
			}
			if got, want := s.job(id).Status, tc.status; got != want {
				t.Fatalf("job status = %s, want %s", got, want)
			}
			if got, want := s.slots(), 1+slotsAfter; got != want {
				t.Fatalf("slots held on model-a = %d, want %d (the bystander plus %d)", got, want, slotsAfter)
			}
			if tc.afterRepair != nil {
				tc.afterRepair(s)
				if got := s.slots(); got != 1 {
					t.Fatalf("slots after the interrupted path continued = %d, want 1", got)
				}
			}
			if j, _, ok := s.next(); !ok || j.ID != "q" {
				t.Fatalf("next() after recovery = (%v, %v), want q", j, ok)
			}
			s.mu.Lock()
			left := len(s.repairs)
			ephemeral := len(s.ephemeral)
			s.mu.Unlock()
			if left != 0 || ephemeral != 0 {
				t.Fatalf("repairs left = %d, ephemeral outcomes left = %d, want none", left, ephemeral)
			}
		})
	}
}

func TestSlotReleaseIsNotRetriedWhileItsRepairIsPending(t *testing.T) {
	s, stub := repairFixture(t)
	stub.setFail(true)
	s.cfg.Instances[0].ServerExecutionLimit = time.Second
	runningJob(s, "j", time.Now().Add(time.Hour))
	s.mu.Lock()
	held := s.jobs["j"]
	held.Status, held.FinishedAt = "failed", time.Now().Add(-time.Minute)
	s.mu.Unlock()
	s.releaseExpiredUncertainSlots(time.Now())
	attempts := stub.attemptCount()
	s.releaseExpiredUncertainSlots(time.Now())
	s.releaseExpiredUncertainSlots(time.Now())
	if stub.attemptCount() != attempts {
		t.Fatal("the natural retry wrote again while the repair is pending")
	}
	stub.setFail(false)
	s.readinessTick(context.Background(), nil)
	s.releaseExpiredUncertainSlots(time.Now())
	if got := s.slots(); got != 1 {
		t.Fatalf("slots = %d, want 1: the slot must be released exactly once", got)
	}
}

func TestUncertainResultKeepsItsSlotAfterRepair(t *testing.T) {
	s, stub := repairFixture(t)
	stub.setFail(true)
	j := runningJob(s, "j", time.Now().Add(-time.Second)) // SlotHeld stays true: uncertain upstream outcome
	if err := s.persistResult(context.Background(), j, 502, errorBody("server_error", "upstream_response_error", "failed"), "failed"); err == nil {
		t.Fatal("persistResult succeeded while the write fails")
	}
	stub.setFail(false)
	s.readinessTick(context.Background(), nil)
	if !s.Ready() {
		t.Fatal("gateway did not become ready")
	}
	if got := s.slots(); got != 2 {
		t.Fatalf("slots = %d, want 2: the slot of an uncertain outcome stays reserved", got)
	}
	if !s.job("j").SlotHeld || s.job("j").Status != "failed" {
		t.Fatalf("stored job = %+v, want failed with the slot held", s.job("j"))
	}
}

func TestRepairDoesNotOverwriteALaterCancellation(t *testing.T) {
	s, stub := repairFixture(t)
	stub.setFail(true)
	j := runningJob(s, "j", time.Now().Add(-time.Second))
	j.SlotHeld = false
	if err := s.persistResult(context.Background(), j, 200, []byte(`{"ok":true}`), "succeeded"); err == nil {
		t.Fatal("persistResult succeeded while the write fails")
	}
	// The client cancels; the cancellation is stored before the repair runs.
	stub.setFail(false)
	s.mu.Lock()
	s.cancelled["j"] = true
	s.mu.Unlock()
	s.persistCancellation("j", "request_canceled")
	s.readinessTick(context.Background(), nil)

	stored := s.job("j")
	if stored.Status != "failed" || stored.Error == nil || stored.Error.Code != "request_canceled" {
		t.Fatalf("stored job = %+v, want the cancellation", stored)
	}
	for _, w := range stub.written("j") {
		if w.Status == "succeeded" {
			t.Fatal("the repair overwrote the cancellation with the result")
		}
	}
	if !s.Ready() {
		t.Fatal("gateway did not become ready after the repair was superseded")
	}
	if got := s.slots(); got != 1 {
		t.Fatalf("slots = %d, want 1: the slot of the superseded result is released once", got)
	}
}

func TestSecondFailureOfTheSameJobKeepsTheSlotObligation(t *testing.T) {
	s, stub := repairFixture(t)
	stub.setFail(true)
	j := runningJob(s, "j", time.Now().Add(-time.Second))
	j.SlotHeld = false
	_ = s.persistResult(context.Background(), j, 200, []byte(`{"ok":true}`), "succeeded")
	s.mu.Lock()
	s.cancelled["j"] = true
	s.mu.Unlock()
	s.persistCancellation("j", "request_canceled") // fails too and replaces the repair
	s.mu.Lock()
	n := len(s.repairs)
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("repairs = %d, want one per job", n)
	}
	stub.setFail(false)
	s.readinessTick(context.Background(), nil)
	if got := s.slots(); got != 1 || !s.Ready() {
		t.Fatalf("slots = %d ready = %v, want 1 and ready", got, s.Ready())
	}
	if s.job("j").Error == nil || s.job("j").Error.Code != "request_canceled" {
		t.Fatalf("stored job = %+v, want the cancellation", s.job("j"))
	}
}

func TestFailedRepairKeepsReadinessOffAcrossTicks(t *testing.T) {
	s, stub := repairFixture(t)
	stub.setFail(true)
	j := runningJob(s, "j", time.Now().Add(-time.Second))
	j.SlotHeld = false
	_ = s.finish(j, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "x")}, "expired")
	for i := 0; i < 3; i++ {
		s.readinessTick(context.Background(), nil)
	}
	if s.Ready() {
		t.Fatal("gateway became ready without writing the state")
	}
	s.readinessTick(context.Background(), errors.New("ping failed"))
	stub.setFail(false)
	s.readinessTick(context.Background(), errors.New("ping failed"))
	if s.Ready() {
		t.Fatal("repair ran although the broker did not answer")
	}
	s.readinessTick(context.Background(), nil)
	if !s.Ready() {
		t.Fatal("gateway did not recover once the broker answered and the write worked")
	}
}
