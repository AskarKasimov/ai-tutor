package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type durableRequest struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	ClientID       string    `json:"client_id"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Hash           string    `json:"hash"`
	Request        Request   `json:"request"`
	SubmittedAt    time.Time `json:"submitted_at"`
	Deadline       time.Time `json:"deadline"`
}
type durableJob struct {
	Version        int             `json:"version"`
	ID             string          `json:"id"`
	ClientID       string          `json:"client_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Hash           string          `json:"hash"`
	Request        Request         `json:"request"`
	SubmittedAt    time.Time       `json:"submitted_at"`
	Deadline       time.Time       `json:"deadline"`
	NotBefore      time.Time       `json:"not_before,omitempty"`
	StartedAt      time.Time       `json:"started_at,omitempty"`
	FinishedAt     time.Time       `json:"finished_at,omitempty"`
	Offset         int64           `json:"offset"`
	Status         string          `json:"status"`
	Attempt        int             `json:"attempt"`
	Instance       string          `json:"instance,omitempty"`
	SlotHeld       bool            `json:"slot_held,omitempty"`
	ProbeReserved  bool            `json:"-"`
	Response       json.RawMessage `json:"response,omitempty"`
	Error          *storedError    `json:"error,omitempty"`
}
type storedError struct {
	Status     int    `json:"status"`
	Type       string `json:"type"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after,omitempty"`
}
type submitResult struct {
	status int
	body   []byte
}

type routeSelection struct {
	complexity string
	instance   string
}

type Service struct {
	cfg               Config
	submitter         *kgo.Client
	stateWriter       *kgo.Client
	ingest            *kgo.GroupTransactSession
	caller            *Caller
	mu                sync.Mutex
	stateMu           sync.Mutex
	workers           sync.WaitGroup
	jobs              map[string]*durableJob
	keys              map[string]string
	accepted          map[string]*durableRequest
	stateIDs          map[string]string
	waiters           map[string]chan struct{}
	activeWaiters     map[string]int
	transportCalls    map[string]map[int]uint64
	routeSelections   map[routeSelection]uint64
	cancelled         map[string]bool
	jobCancels        map[string]context.CancelFunc
	ephemeral         map[string]submitResult
	inFlight          map[string]int
	reserved          int
	reservedBytes     int64
	bytes             int64
	connections       chan struct{}
	wake              chan struct{}
	roundRobin        map[string]int
	expected          map[string]float64   // moving average of each instance's upstream call time (spec, section 5)
	cooling           map[string]time.Time // until when an instance that failed goes after healthy ones
	ready             bool
	brokerAvailable   bool
	stateUncertain    bool
	stopping          bool
	started           time.Time
	idempotentReplays uint64
}

func NewService(cfg Config) (*Service, error) {
	cfg.defaults()
	s := &Service{cfg: cfg, jobs: map[string]*durableJob{}, keys: map[string]string{}, accepted: map[string]*durableRequest{}, stateIDs: map[string]string{}, waiters: map[string]chan struct{}{}, activeWaiters: map[string]int{}, cancelled: map[string]bool{}, jobCancels: map[string]context.CancelFunc{}, ephemeral: map[string]submitResult{}, inFlight: map[string]int{}, expected: map[string]float64{}, routeSelections: map[routeSelection]uint64{}, connections: make(chan struct{}, cfg.MaxConnections), wake: make(chan struct{}, 1), roundRobin: map[string]int{}, started: time.Now()}
	brokers := cfg.KafkaBrokers
	var err error
	s.submitter, err = kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.RecordRetries(3), kgo.ProducerLinger(5*time.Millisecond), kgo.MaxBufferedRecords(cfg.MaxConnections), kgo.ProducerBatchMaxBytes(2<<20))
	if err != nil {
		return nil, fmt.Errorf("create request producer: %w", err)
	}
	s.stateWriter, err = kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.TransactionalID(cfg.TransactionalID+"-state"), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.RecordRetries(3), kgo.ProducerBatchMaxBytes(40<<20))
	if err != nil {
		s.submitter.Close()
		return nil, fmt.Errorf("create state producer: %w", err)
	}
	s.ingest, err = kgo.NewGroupTransactSession(kgo.SeedBrokers(brokers...), kgo.TransactionalID(cfg.TransactionalID+"-ingest"), kgo.ConsumerGroup(cfg.ConsumerGroup), kgo.ConsumeTopics(cfg.RequestsTopic), kgo.ConsumeStartOffset(kgo.NewOffset().AtStart()), kgo.ConsumeResetOffset(kgo.NoResetOffset()), kgo.FetchIsolationLevel(kgo.ReadCommitted()), kgo.DisableAutoCommit(), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.ProducerBatchMaxBytes(40<<20))
	if err != nil {
		s.submitter.Close()
		s.stateWriter.Close()
		return nil, fmt.Errorf("create request consumer: %w", err)
	}
	s.caller = NewCaller(cfg)
	return s, nil
}

func (s *Service) Close() {
	s.mu.Lock()
	s.stopping = true
	s.ready = false
	s.mu.Unlock()
	s.ingest.Close()
	s.stateWriter.Close()
	s.submitter.Close()
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.restore(ctx); err != nil {
		return err
	}
	monitorCtx, stopMonitor := context.WithCancel(ctx)
	defer stopMonitor()
	go s.monitorReadiness(monitorCtx)
	s.mu.Lock()
	s.brokerAvailable = true
	s.ready = true
	s.mu.Unlock()
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	consumeDone := make(chan struct{})
	go func() { defer close(consumeDone); s.consume(ctx) }()
	dispatchDone := make(chan struct{})
	go func() { defer close(dispatchDone); s.dispatch(ctx, workCtx) }()
	go s.compactLoop(ctx)
	<-ctx.Done()
	s.mu.Lock()
	s.ready = false
	s.stopping = true
	s.mu.Unlock()
	<-dispatchDone
	<-consumeDone
	done := make(chan struct{})
	go func() { s.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(s.cfg.GracePeriod):
		cancelWork()
		<-done
	}
	return nil
}

func (s *Service) monitorReadiness(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			err := s.submitter.Ping(pingCtx)
			cancel()
			s.mu.Lock()
			s.brokerAvailable = err == nil
			lagged := false
			for _, request := range s.accepted {
				if time.Since(request.SubmittedAt) > s.cfg.MaxInputLag {
					lagged = true
					break
				}
			}
			if !s.stopping {
				s.ready = err == nil && !lagged && !s.stateUncertain
			}
			s.mu.Unlock()
			if err == nil {
				s.signal()
			}
		}
	}
}

func (s *Service) restore(ctx context.Context) error {
	reader, err := kgo.NewClient(kgo.SeedBrokers(s.cfg.KafkaBrokers...), kgo.FetchIsolationLevel(kgo.ReadCommitted()), kgo.ConsumeResetOffset(kgo.NoResetOffset()), kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{s.cfg.StateTopic: {0: kgo.NewOffset().AtStart()}}), kgo.FetchMaxWait(100*time.Millisecond))
	if err != nil {
		return fmt.Errorf("create state restore reader: %w", err)
	}
	defer reader.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	err = reader.Ping(pingCtx)
	pingCancel()
	if err != nil {
		return fmt.Errorf("connect to Redpanda before state recovery: %w", err)
	}
	barrierValue, err := s.publishRestoreBarrier(ctx)
	if err != nil {
		return fmt.Errorf("publish gateway state recovery barrier: %w", err)
	}
	for {
		c, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		fs := reader.PollFetches(c)
		cancel()
		for _, e := range fs.Errors() {
			if errors.Is(e.Err, context.DeadlineExceeded) || errors.Is(e.Err, context.Canceled) {
				continue
			}
			return fmt.Errorf("read gateway state: %w", e.Err)
		}
		barrierSeen := false
		for it := fs.RecordIter(); !it.Done(); {
			r := it.Next()
			if string(r.Key) == restoreBarrierKey {
				if isRestoreBarrier(r, barrierValue) {
					barrierSeen = true
					break
				}
				continue
			}
			if len(r.Value) == 0 {
				s.removeByStateKey(r.Key)
				continue
			}
			var j durableJob
			if err := json.Unmarshal(r.Value, &j); err != nil {
				return fmt.Errorf("decode gateway state: %w", err)
			}
			s.install(&j)
		}
		if barrierSeen {
			s.mu.Lock()
			var recoveries []*durableJob
			for _, j := range s.jobs {
				if j.Error != nil && j.Error.Code == "execution_interrupted" || j.Status == "queued" {
					copy := *j
					if copy.Status == "queued" {
						if !time.Now().Before(copy.Deadline) {
							copy.Status = "expired"
							copy.Error = &storedError{Status: 504, Type: "gateway_timeout", Code: "request_deadline_exceeded", Message: "LLM request deadline exceeded."}
						} else {
							copy.Status = "failed"
							copy.Error = &storedError{Status: 409, Type: "request_cancelled", Code: "client_disconnected", Message: "No client remains connected after gateway restart."}
						}
					}
					recoveries = append(recoveries, &copy)
				}
			}
			s.mu.Unlock()
			for _, job := range recoveries {
				if err := s.save(ctx, job); err != nil {
					return fmt.Errorf("persist recovered gateway state: %w", err)
				}
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

const restoreBarrierKey = "\x00llm-gateway-restore-barrier"

func isRestoreBarrier(record *kgo.Record, value string) bool {
	return record != nil && string(record.Key) == restoreBarrierKey && string(record.Value) == value
}

func (s *Service) publishRestoreBarrier(ctx context.Context) (string, error) {
	value := newID()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if err := s.stateWriter.BeginTransaction(); err != nil {
		return "", err
	}
	result := s.stateWriter.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: []byte(restoreBarrierKey), Value: []byte(value)})
	if err := result.FirstErr(); err != nil {
		_ = s.stateWriter.EndTransaction(context.Background(), kgo.TryAbort)
		return "", err
	}
	if err := s.stateWriter.EndTransaction(ctx, kgo.TryCommit); err != nil {
		return "", err
	}
	return value, nil
}

func (s *Service) install(j *durableJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expiredForRetention(j, time.Now(), s.cfg.Retention) {
		s.forgetExpiredJobLocked(j.ID)
		return
	}
	if old := s.jobs[j.ID]; old != nil {
		if isPending(old.Status) {
			s.bytes -= int64(len(mustJSON(old.Request)))
		}
		if old.SlotHeld && old.Instance != "" && s.inFlight[old.Instance] > 0 {
			s.inFlight[old.Instance]--
		}
	}
	copy := *j
	if copy.Status == "running" {
		copy.Status = "failed"
		copy.Error = &storedError{Status: 502, Type: "server_error", Code: "execution_interrupted", Message: "LLM execution was interrupted by gateway restart."}
		copy.SlotHeld = true
		copy.FinishedAt = time.Now()
	}
	if !isPending(copy.Status) && copy.SlotHeld && (s.upstreamAbortsOnDisconnect(copy.Instance) || s.uncertainSlotExpired(&copy, time.Now())) {
		// A verified disconnect-abort contract or server bound can release it.
		copy.SlotHeld = false
	}
	s.jobs[j.ID] = &copy
	s.stateIDs[string(stateKey(&copy))] = copy.ID
	if isPending(copy.Status) {
		s.bytes += int64(len(mustJSON(copy.Request)))
	}
	if accepted := s.accepted[j.ID]; accepted != nil {
		delete(s.accepted, j.ID)
		s.reserved--
		s.reservedBytes -= int64(len(mustJSON(accepted)))
	}
	if copy.IdempotencyKey != "" {
		s.keys[identity(copy.ClientID, copy.IdempotencyKey)] = copy.ID
	}
	if copy.SlotHeld && copy.Instance != "" {
		s.inFlight[copy.Instance]++
	}
}

// forgetExpiredJobLocked removes all process-local state for a key whose
// retention has elapsed, including transient cancellation state that could
// otherwise poison a new request reusing the same stable job ID.
func (s *Service) forgetExpiredJobLocked(id string) {
	if old := s.jobs[id]; old != nil {
		if isPending(old.Status) {
			s.bytes -= int64(len(mustJSON(old.Request)))
		}
		if old.SlotHeld && old.Instance != "" && s.inFlight[old.Instance] > 0 {
			s.inFlight[old.Instance]--
		}
		delete(s.jobs, id)
	}
	for key, jobID := range s.stateIDs {
		if jobID == id {
			delete(s.stateIDs, key)
		}
	}
	for key, jobID := range s.keys {
		if jobID == id {
			delete(s.keys, key)
		}
	}
	delete(s.cancelled, id)
}

func (s *Service) removeByStateKey(key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.stateIDs[string(key)]
	if id == "" {
		return
	}
	j := s.jobs[id]
	if j != nil {
		if isPending(j.Status) {
			s.bytes -= int64(len(mustJSON(j.Request)))
		}
		if j.SlotHeld && j.Instance != "" && s.inFlight[j.Instance] > 0 {
			s.inFlight[j.Instance]--
		}
		if j.IdempotencyKey != "" {
			index := identity(j.ClientID, j.IdempotencyKey)
			if s.keys[index] == id {
				delete(s.keys, index)
			}
		}
		delete(s.jobs, id)
	}
	delete(s.stateIDs, string(key))
}

func (s *Service) consume(ctx context.Context) {
	defer s.ingest.Close()
	for ctx.Err() == nil {
		c, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		fs := s.ingest.PollRecords(c, 1)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if fs.Empty() {
			continue
		}
		errs := fs.Errors()
		if len(errs) > 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		it := fs.RecordIter()
		if it.Done() {
			continue
		}
		record := it.Next()
		var request durableRequest
		if json.Unmarshal(record.Value, &request) != nil {
			continue
		}
		s.mu.Lock()
		duplicate := s.cancelled[request.ID]
		if request.IdempotencyKey != "" {
			if existingID, ok := s.keys[identity(request.ClientID, request.IdempotencyKey)]; ok && existingID != request.ID {
				duplicate = true
			}
		}
		if _, ok := s.jobs[request.ID]; ok {
			duplicate = true
		}
		orphaned := !duplicate && (s.accepted[request.ID] == nil || s.activeWaiters[request.ID] == 0)
		canceled := s.cancelled[request.ID]
		s.mu.Unlock()
		s.stateMu.Lock()
		if err := s.ingest.Begin(); err != nil {
			s.stateMu.Unlock()
			time.Sleep(time.Second)
			continue
		}
		var job *durableJob
		if !duplicate {
			job = &durableJob{Version: 1, ID: request.ID, ClientID: request.ClientID, IdempotencyKey: request.IdempotencyKey, Hash: request.Hash, Request: request.Request, SubmittedAt: request.SubmittedAt, Deadline: request.Deadline, Offset: record.Offset, Status: "queued"}
			if orphaned {
				code, message := "client_disconnected", "No client remains connected to receive this request."
				if canceled {
					code, message = "request_canceled", "The request was canceled because no clients are waiting."
				}
				job.Status = "failed"
				job.FinishedAt = time.Now()
				job.Error = &storedError{Status: 409, Type: "request_cancelled", Code: code, Message: message}
			}
			raw, _ := json.Marshal(job)
			if got := s.ingest.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: stateKey(job), Value: raw}); got.FirstErr() != nil {
				_, _ = s.ingest.End(context.Background(), kgo.TryAbort)
				s.stateMu.Unlock()
				time.Sleep(time.Second)
				continue
			}
		}
		committed, err := s.ingest.End(context.Background(), kgo.TryCommit)
		if err != nil || !committed {
			s.stateMu.Unlock()
			continue
		}
		if job != nil {
			s.install(job)
			if job.Status == "queued" {
				s.signal()
			}
		}
		s.stateMu.Unlock()
	}
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) dispatch(ctx, workCtx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-time.After(time.Second):
		}
		for {
			s.releaseExpiredUncertainSlots(time.Now())
			s.expireQueued(time.Now())
			j, in, ok := s.next()
			if !ok {
				break
			}
			if time.Now().After(j.Deadline) {
				j.SlotHeld = false
				s.finish(j, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "LLM request deadline exceeded.")}, "expired")
				s.releaseSlot(j)
				continue
			}
			j.Status = "running"
			if j.StartedAt.IsZero() {
				j.StartedAt = time.Now()
			}
			j.Attempt++
			j.Instance = in.ID
			j.SlotHeld = true
			if err := s.save(ctx, j); err != nil {
				s.rollbackDispatch(j)
				break
			}
			if time.Now().After(j.Deadline) {
				s.finish(j, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "LLM request deadline exceeded.")}, "expired")
				s.releaseSlot(j)
				continue
			}
			jobCtx, jobCancel := context.WithCancel(workCtx)
			s.mu.Lock()
			s.jobCancels[j.ID] = jobCancel
			if s.cancelled[j.ID] {
				jobCancel()
			}
			s.mu.Unlock()
			s.workers.Add(1)
			go func() {
				defer s.workers.Done()
				defer jobCancel()
				defer func() { s.mu.Lock(); delete(s.jobCancels, j.ID); s.mu.Unlock() }()
				s.execute(jobCtx, j, in)
			}()
		}
	}
}

func (s *Service) rollbackDispatch(j *durableJob) {
	if j.ProbeReserved {
		s.caller.circuit(j.Instance).releaseReservation()
	}
	s.mu.Lock()
	if current := s.jobs[j.ID]; current != nil && current.Status == "running" {
		current.Status = "queued"
		current.Instance = ""
		current.SlotHeld = false
	}
	if s.inFlight[j.Instance] > 0 {
		s.inFlight[j.Instance]--
	}
	s.mu.Unlock()
	s.signal()
}

func (s *Service) expireQueued(now time.Time) {
	s.mu.Lock()
	var expired []*durableJob
	for _, job := range s.jobs {
		if job.Status == "queued" && !now.Before(job.Deadline) {
			job.Status = "expiring"
			copy := *job
			expired = append(expired, &copy)
		}
	}
	s.mu.Unlock()
	for _, job := range expired {
		if !s.finish(job, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "LLM request deadline exceeded.")}, "expired") {
			s.mu.Lock()
			if current := s.jobs[job.ID]; current != nil && current.Status == "expiring" {
				current.Status = "queued"
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) next() (*durableJob, InstanceConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return nil, InstanceConfig{}, false
	}
	if !s.brokerAvailable || s.stateUncertain {
		return nil, InstanceConfig{}, false
	}
	var pending []*durableJob
	for _, j := range s.jobs {
		if j.Status == "queued" && !s.cancelled[j.ID] {
			pending = append(pending, j)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Offset < pending[j].Offset })
	for pass := 0; pass < 2; pass++ {
		ordered := pending
		if pass == 1 {
			ordered = append([]*durableJob(nil), pending...)
			sort.SliceStable(ordered, func(i, j int) bool {
				ri := complexityRank(ordered[i].Request.Complexity)
				rj := complexityRank(ordered[j].Request.Complexity)
				if ri != rj {
					return ri > rj
				}
				return ordered[i].Offset < ordered[j].Offset
			})
		}
		for _, job := range ordered {
			if time.Now().Before(job.NotBefore) {
				continue
			}
			if (pass == 0) != job.Request.IsUrgent() {
				continue
			}
			pool := s.eligibleInstances(job.Request.Complexity)
			var candidates []InstanceConfig
			for _, in := range pool {
				if s.inFlight[in.ID] >= in.MaxInFlight || !supports(in, job.Request) || !s.caller.circuit(in.ID).available(time.Now()) {
					continue
				}
				candidates = append(candidates, in)
			}
			if len(candidates) == 0 {
				continue
			}
			// The free instance that answers soonest: least loaded, then the
			// shortest expected call (an instance without samples first, so
			// every one is tried), then the less capable tier, then round-robin.
			// An instance cooling down after a failure goes after the healthy
			// ones, so a model that keeps answering 429 cannot keep the "not
			// measured yet" priority.
			now := time.Now()
			rank := func(in InstanceConfig) (bool, float64, float64, int) {
				return now.Before(s.cooling[in.ID]), float64(s.inFlight[in.ID]) / float64(in.MaxInFlight), s.expected[in.ID], s.instanceTier(in.ID)
			}
			sort.SliceStable(candidates, func(a, b int) bool {
				ca, ra, ea, ta := rank(candidates[a])
				cb, rb, eb, tb := rank(candidates[b])
				if ca != cb {
					return !ca
				}
				if ra != rb {
					return ra < rb
				}
				if ea != eb {
					return ea < eb
				}
				return ta < tb
			})
			bestCooling, bestRatio, bestExpected, bestTier := rank(candidates[0])
			ties := make([]InstanceConfig, 0, len(candidates))
			for _, candidate := range candidates {
				if c, r, e, t := rank(candidate); c == bestCooling && r == bestRatio && e == bestExpected && t == bestTier {
					ties = append(ties, candidate)
				}
			}
			cursor := s.roundRobin[job.Request.Complexity] % len(ties)
			var selected InstanceConfig
			probeReserved := false
			for n := 0; n < len(ties); n++ {
				candidate := ties[(cursor+n)%len(ties)]
				ok, probe := s.caller.circuit(candidate.ID).reserve(time.Now())
				if ok {
					selected = candidate
					probeReserved = probe
					s.roundRobin[job.Request.Complexity] = (cursor + n + 1) % len(ties)
					break
				}
			}
			if selected.ID == "" {
				continue
			}
			s.inFlight[selected.ID]++
			if s.routeSelections == nil {
				s.routeSelections = make(map[routeSelection]uint64)
			}
			s.routeSelections[routeSelection{complexity: job.Request.Complexity, instance: selected.ID}]++
			job.Status = "running"
			job.Instance = selected.ID
			job.SlotHeld = true
			j := *job
			j.Instance = selected.ID
			j.SlotHeld = true
			j.ProbeReserved = probeReserved
			return &j, selected, true
		}
	}
	return nil, InstanceConfig{}, false
}

func (c *Circuit) available(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failures < 3 || !now.Before(c.openUntil) && !c.probing
}
func (c *Circuit) reserve(now time.Time) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures < 3 {
		return true, false
	}
	if now.Before(c.openUntil) || c.probing {
		return false, false
	}
	c.probing = true
	return true, true
}
func (c *Circuit) releaseReservation() { c.mu.Lock(); c.probing = false; c.mu.Unlock() }
func (s *Service) instance(id string) InstanceConfig {
	for _, in := range s.cfg.Instances {
		if in.ID == id {
			return in
		}
	}
	return InstanceConfig{}
}

var complexityLevels = [...]string{"low", "medium", "high"}

func complexityRank(complexity string) int {
	for rank, level := range complexityLevels {
		if complexity == level {
			return rank
		}
	}
	return -1
}

func (s *Service) instanceTier(id string) int {
	tier := -1
	for complexity, instances := range s.cfg.Pools {
		for _, candidate := range instances {
			if candidate == id && complexityRank(complexity) > tier {
				tier = complexityRank(complexity)
			}
		}
	}
	return tier
}

func (s *Service) eligibleInstances(complexity string) []InstanceConfig {
	rank := complexityRank(complexity)
	if rank < 0 {
		return nil
	}
	seen := make(map[string]bool)
	var instances []InstanceConfig
	for _, level := range complexityLevels {
		if complexityRank(level) < rank {
			continue
		}
		for _, id := range s.cfg.Pools[level] {
			if seen[id] {
				continue
			}
			seen[id] = true
			if instance := s.instance(id); instance.ID != "" {
				instances = append(instances, instance)
			}
		}
	}
	return instances
}

func supports(in InstanceConfig, r Request) bool {
	if !supportsParameters(in, r) {
		return false
	}
	promptTokens := int64(0)
	for _, m := range r.Messages {
		promptTokens += int64(len(m.Content))/2 + 8
	}
	remaining := int64(in.ContextWindow) - promptTokens
	if remaining < 0 {
		return false
	}
	for _, name := range []string{"max_tokens", "max_completion_tokens"} {
		raw, ok := r.Parameters[name]
		if !ok {
			continue
		}
		var budget int64
		if err := json.Unmarshal(raw, &budget); err != nil || budget <= 0 || budget > remaining {
			return false
		}
		remaining -= budget
	}
	return true
}

func supportsParameters(in InstanceConfig, r Request) bool {
	supported := map[string]bool{}
	for _, p := range in.SupportedParameters {
		supported[p] = true
	}
	for p := range r.Parameters {
		if !supported[p] {
			return false
		}
		if p == "response_format" {
			var format struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(r.Parameters[p], &format) != nil {
				return false
			}
			known := false
			for _, value := range in.SupportedResponseFormats {
				if value == format.Type {
					known = true
					break
				}
			}
			if !known {
				return false
			}
		}
	}
	return true
}

func (s *Service) execute(ctx context.Context, j *durableJob, in InstanceConfig) {
	if time.Now().After(j.Deadline) {
		if j.ProbeReserved {
			s.caller.circuit(in.ID).releaseReservation()
		}
		j.SlotHeld = false
		s.finish(j, submitResult{504, errorBody("gateway_timeout", "request_deadline_exceeded", "LLM request deadline exceeded.")}, "expired")
		s.releaseSlot(j)
		return
	}
	callCtx, cancel := context.WithDeadline(ctx, j.Deadline)
	started := time.Now()
	result, failure := s.caller.call(callCtx, in, j.Request, j.ID, j.ProbeReserved)
	cancel()
	s.observeCall(j, in, failure, time.Since(started))
	if failure != nil {
		if s.isCanceled(j.ID) {
			s.releaseSlot(j)
			return
		}
		if j.Attempt < 2 && (failure.BeforeSend || failure.Status == 429) && time.Now().Add(failure.RetryAfter).Before(j.Deadline) {
			s.releaseSlot(j)
			j.Status = "queued"
			j.SlotHeld = false
			j.Instance = ""
			j.NotBefore = time.Now().Add(failure.RetryAfter)
			j.ProbeReserved = false
			if err := s.save(ctx, j); err == nil {
				s.signal()
			} else if !errors.Is(err, errCanceledJob) {
				// The durable state is still "running". Do not dispatch more work
				// while local state claims this retry is queued.
				s.markStateUncertain()
				s.notifyEphemeral(j.ID, submitResult{503, errorBody("server_error", "state_unavailable", "Gateway could not durably reschedule the LLM request.")})
			}
			return
		}
		status := 502
		typ := "server_error"
		code := failure.Code
		msg := "The upstream LLM request failed."
		if failure.Status == 429 {
			status = 429
			typ = "rate_limit_error"
			code = "upstream_rate_limit"
			msg = "The upstream LLM rate limit was reached."
		}
		if errors.Is(failure.Err, context.DeadlineExceeded) {
			status = 504
			typ = "gateway_timeout"
			code = "upstream_timeout"
			msg = "The upstream LLM request timed out."
		}
		body := errorBody(typ, code, msg)
		uncertain := uncertainUpstreamOutcomeForInstance(in, failure)
		if !uncertain {
			j.SlotHeld = false
		}
		if err := s.persistResult(ctx, j, status, body, "failed", failure.RetryAfter); err == nil || errors.Is(err, errCanceledJob) {
			if !uncertain || errors.Is(err, errCanceledJob) {
				s.releaseSlot(j)
			}
		}
		return
	}
	if s.isCanceled(j.ID) {
		s.releaseSlot(j)
		return
	}
	if int64(len(result.Body)) > 32<<20 {
		j.SlotHeld = false
		_ = s.persistResult(ctx, j, 502, errorBody("server_error", "upstream_response_too_large", "The upstream response exceeded the gateway limit."), "failed")
		s.releaseSlot(j)
		return
	}
	j.SlotHeld = false
	if err := s.persistResult(ctx, j, 200, result.Body, "succeeded"); err == nil || errors.Is(err, errCanceledJob) {
		s.releaseSlot(j)
	}
}

// expectedAlpha weighs the newest call in an instance's expected duration.
const expectedAlpha = 0.3

// failureCooldown is how long a failed instance goes after healthy ones when
// the upstream gave no Retry-After; it matches the breaker pause.
const failureCooldown = 15 * time.Second

// observeCall learns from a finished call. A success and an upstream timeout
// are duration samples (a timeout marks a slow instance). A 429, a 5xx or a
// transport failure puts the instance on cooldown instead: they say nothing
// about speed, but without it an unmeasured instance that keeps failing would
// keep the first place. A cancelled request and an open breaker change nothing.
func (s *Service) observeCall(j *durableJob, in InstanceConfig, failure *UpstreamFailure, elapsed time.Duration) {
	if s.isCanceled(j.ID) || failure != nil && (failure.Code == "request_canceled" || failure.Code == "upstream_circuit_open") {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expected == nil {
		s.expected = make(map[string]float64)
	}
	if s.cooling == nil {
		s.cooling = make(map[string]time.Time)
	}
	sample := func() {
		seconds := elapsed.Seconds()
		if previous, ok := s.expected[in.ID]; ok {
			seconds = expectedAlpha*seconds + (1-expectedAlpha)*previous
		}
		s.expected[in.ID] = seconds
	}
	timedOut := failure != nil && errors.Is(failure.Err, context.DeadlineExceeded)
	switch {
	case failure == nil:
		sample()
		delete(s.cooling, in.ID)
	case failure.BeforeSend || failure.Status == 429 || failure.Status >= 500 || failure.Status == 0 && !timedOut:
		cooldown := failure.RetryAfter
		if cooldown <= 0 {
			cooldown = failureCooldown
		}
		s.cooling[in.ID] = time.Now().Add(cooldown)
	case timedOut:
		sample()
	}
}

func uncertainUpstreamOutcome(failure *UpstreamFailure) bool {
	if failure == nil {
		return false
	}
	return errors.Is(failure.Err, context.DeadlineExceeded) || failure.Code == "upstream_response_error" || (failure.Code == "upstream_transport_error" && !failure.BeforeSend)
}

func uncertainUpstreamOutcomeForInstance(in InstanceConfig, failure *UpstreamFailure) bool {
	if !uncertainUpstreamOutcome(failure) {
		return false
	}
	// The HTTP transport closes its request when its deadline fires. Only trust
	// this as a completed upstream cancellation for explicitly verified models.
	return !(in.AbortOnDisconnect && errors.Is(failure.Err, context.DeadlineExceeded))
}

func (s *Service) persistResult(ctx context.Context, j *durableJob, status int, body []byte, state string, retryAfter ...time.Duration) error {
	j.Status = state
	j.FinishedAt = time.Now()
	if status == 200 {
		j.Response = append([]byte(nil), body...)
	} else {
		var envelope ErrorEnvelope
		_ = json.Unmarshal(body, &envelope)
		j.Error = &storedError{Status: status, Type: envelope.Error.Type, Code: envelope.Error.Code, Message: envelope.Error.Message}
		if len(retryAfter) > 0 && retryAfter[0] > 0 {
			j.Error.RetryAfter = int(math.Ceil(retryAfter[0].Seconds()))
		}
	}
	for {
		if err := s.save(ctx, j); err == nil {
			break
		} else {
			if errors.Is(err, errCanceledJob) {
				return err
			}
			if time.Now().After(j.Deadline) || ctx.Err() != nil {
				s.markStateUncertain()
				s.notifyEphemeral(j.ID, submitResult{503, errorBody("server_error", "state_unavailable", "Gateway could not durably save the LLM result.")})
				return err
			}
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-time.After(time.Until(j.Deadline)):
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	s.mu.Lock()
	delete(s.ephemeral, j.ID)
	if ch := s.waiters[j.ID]; ch != nil {
		close(ch)
		delete(s.waiters, j.ID)
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) notifyEphemeral(id string, result submitResult) {
	s.mu.Lock()
	s.ephemeral[id] = result
	if ch := s.waiters[id]; ch != nil {
		close(ch)
		delete(s.waiters, id)
	}
	s.mu.Unlock()
}

func (s *Service) compactLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.compactExpired(ctx, now)
		}
	}
}

func (s *Service) compactExpired(ctx context.Context, now time.Time) {
	s.mu.Lock()
	var expired []*durableJob
	for _, j := range s.jobs {
		if expiredForRetention(j, now, s.cfg.Retention) {
			copy := *j
			expired = append(expired, &copy)
		}
	}
	s.mu.Unlock()
	for _, j := range expired {
		s.stateMu.Lock()
		s.mu.Lock()
		current := s.jobs[j.ID]
		stillExpired := current != nil && current.ID == j.ID && expiredForRetention(current, now, s.cfg.Retention)
		s.mu.Unlock()
		if !stillExpired {
			s.stateMu.Unlock()
			continue
		}
		if err := s.stateWriter.BeginTransaction(); err != nil {
			s.stateMu.Unlock()
			continue
		}
		result := s.stateWriter.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: stateKey(j), Value: nil})
		if result.FirstErr() != nil {
			_ = s.stateWriter.EndTransaction(context.Background(), kgo.TryAbort)
			s.stateMu.Unlock()
			continue
		}
		if err := s.stateWriter.EndTransaction(ctx, kgo.TryCommit); err != nil {
			s.stateMu.Unlock()
			continue
		}
		s.mu.Lock()
		if current := s.jobs[j.ID]; current != nil && expiredForRetention(current, now, s.cfg.Retention) {
			s.forgetExpiredJobLocked(j.ID)
		}
		s.mu.Unlock()
		s.stateMu.Unlock()
	}
}
func (s *Service) finish(j *durableJob, result submitResult, state string) bool {
	j.Status = state
	j.FinishedAt = time.Now()
	var e ErrorEnvelope
	_ = json.Unmarshal(result.body, &e)
	j.Error = &storedError{Status: result.status, Type: e.Error.Type, Code: e.Error.Code, Message: e.Error.Message}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.save(ctx, j) == nil {
		s.mu.Lock()
		if ch := s.waiters[j.ID]; ch != nil {
			close(ch)
			delete(s.waiters, j.ID)
		}
		s.mu.Unlock()
		return true
	} else {
		s.markStateUncertain()
		s.notifyEphemeral(j.ID, submitResult{503, errorBody("server_error", "state_unavailable", "Gateway could not durably save the request state.")})
		return false
	}
}
func (s *Service) releaseSlot(j *durableJob) {
	if j.Instance == "" {
		return
	}
	s.mu.Lock()
	if s.inFlight[j.Instance] > 0 {
		s.inFlight[j.Instance]--
	}
	j.SlotHeld = false
	if saved := s.jobs[j.ID]; saved != nil {
		saved.SlotHeld = false
	}
	s.mu.Unlock()
	s.signal()
}

func (s *Service) uncertainSlotExpired(j *durableJob, now time.Time) bool {
	if j == nil || !j.SlotHeld || j.FinishedAt.IsZero() {
		return false
	}
	for _, in := range s.cfg.Instances {
		if in.ID == j.Instance {
			if in.ServerExecutionLimit > 0 {
				// Reserve a full server window after the last uncertain outcome. The
				// first attempt's StartedAt cannot bound a later retry or time in IO.
				return !now.Before(j.FinishedAt.Add(in.ServerExecutionLimit))
			}
			return false
		}
	}
	return false
}

func (s *Service) upstreamAbortsOnDisconnect(instanceID string) bool {
	for _, in := range s.cfg.Instances {
		if in.ID == instanceID {
			return in.AbortOnDisconnect
		}
	}
	return false
}

func (s *Service) releaseExpiredUncertainSlots(now time.Time) {
	s.mu.Lock()
	var expired []*durableJob
	for _, job := range s.jobs {
		if !isPending(job.Status) && s.uncertainSlotExpired(job, now) {
			copy := *job
			copy.SlotHeld = false
			expired = append(expired, &copy)
		}
	}
	s.mu.Unlock()

	for _, job := range expired {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.save(ctx, job)
		cancel()
		if err != nil {
			if !errors.Is(err, errCanceledJob) {
				s.markStateUncertain()
			}
			continue
		}
		s.releaseSlot(job)
	}
}

func (s *Service) save(ctx context.Context, j *durableJob) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.mu.Lock()
	current := s.jobs[j.ID]
	if s.cancelled[j.ID] && (j.Error == nil || j.Error.Code != "request_canceled") {
		s.mu.Unlock()
		return errCanceledJob
	}
	if current != nil && !isPending(current.Status) {
		currentCode, nextCode := "", ""
		if current.Error != nil {
			currentCode = current.Error.Code
		}
		if j.Error != nil {
			nextCode = j.Error.Code
		}
		if current.Status != j.Status || currentCode != nextCode {
			s.mu.Unlock()
			return errCanceledJob
		}
	}
	s.mu.Unlock()
	copy := *j
	raw, err := json.Marshal(copy)
	if err != nil {
		return err
	}
	if err = s.stateWriter.BeginTransaction(); err != nil {
		return err
	}
	res := s.stateWriter.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: stateKey(j), Value: raw})
	if err = res.FirstErr(); err != nil {
		abortCtx, stopAbort := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.stateWriter.EndTransaction(abortCtx, kgo.TryAbort)
		stopAbort()
		return err
	}
	if err = s.stateWriter.EndTransaction(ctx, kgo.TryCommit); err != nil {
		return err
	}
	s.mu.Lock()
	old := s.jobs[j.ID]
	if old == nil {
		if isPending(j.Status) {
			s.bytes += int64(len(mustJSON(j.Request)))
		}
	} else {
		if isPending(old.Status) {
			s.bytes -= int64(len(mustJSON(old.Request)))
		}
		if isPending(j.Status) {
			s.bytes += int64(len(mustJSON(j.Request)))
		}
	}
	if !isPending(j.Status) {
		if accepted := s.accepted[j.ID]; accepted != nil {
			delete(s.accepted, j.ID)
			s.reserved--
			s.reservedBytes -= int64(len(mustJSON(accepted)))
		}
	}
	saved := *j
	s.jobs[j.ID] = &saved
	s.stateIDs[string(stateKey(&saved))] = saved.ID
	if saved.IdempotencyKey != "" {
		s.keys[identity(saved.ClientID, saved.IdempotencyKey)] = saved.ID
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health/live" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.URL.Path == "/health/ready" && r.Method == http.MethodGet {
		s.mu.Lock()
		ready := s.ready && !s.stopping
		s.mu.Unlock()
		if !ready {
			http.Error(w, "not ready", 503)
			return
		}
		w.WriteHeader(200)
		return
	}
	if r.URL.Path == "/metrics" && r.Method == http.MethodGet {
		s.writeMetrics(w)
		return
	}
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		writeError(w, 404, "invalid_request_error", "not_found", "Not found.")
		return
	}
	w.Header().Set("X-Request-ID", newID())
	select {
	case s.connections <- struct{}{}:
		defer func() { <-s.connections }()
	default:
		s.recordCall("http", 429)
		writeError(w, 429, "rate_limit_error", "connection_limit", "Too many concurrent requests.")
		return
	}
	if !s.Ready() {
		s.recordCall("http", 503)
		writeError(w, 503, "server_error", "gateway_not_ready", "Gateway is not ready.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	body, err := ioReadAll(r.Body)
	if err != nil {
		s.recordCall("http", 413)
		writeError(w, 413, "invalid_request_error", "request_too_large", "Request body is too large.")
		return
	}
	req, err := ParseRequest(body)
	if err != nil {
		s.recordCall("http", 400)
		writeStatusError(w, err)
		return
	}
	result, id, err := s.Complete(r.Context(), req, strings.TrimSpace(r.Header.Get("Idempotency-Key")), "http", true, func(id string) { w.Header().Set("X-Request-ID", id) })
	w.Header().Set("X-Request-ID", id)
	if err != nil {
		writeStatusError(w, err)
		return
	}
	if result.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(result.retryAfter))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.status)
	_, _ = w.Write(result.body)
}

func (s *Service) retryPublish(payload durableRequest, raw []byte, size int64) {
	ctx, cancel := context.WithDeadline(context.Background(), payload.Deadline)
	defer cancel()
	for {
		if ctx.Err() != nil || s.isCanceled(payload.ID) {
			break
		}
		res := s.submitter.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.RequestsTopic, Key: []byte(payload.ID), Value: raw})
		if res.FirstErr() == nil {
			return
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	if s.isCanceled(payload.ID) {
		return
	}
	s.mu.Lock()
	if _, stored := s.jobs[payload.ID]; !stored {
		if _, ok := s.accepted[payload.ID]; ok {
			delete(s.accepted, payload.ID)
			s.reserved--
			s.reservedBytes -= size
			if payload.IdempotencyKey != "" && s.keys[identity(payload.ClientID, payload.IdempotencyKey)] == payload.ID {
				delete(s.keys, identity(payload.ClientID, payload.IdempotencyKey))
			}
			delete(s.waiters, payload.ID)
		}
	}
	s.mu.Unlock()
}

func (s *Service) validatePool(r Request) error {
	parametersSupported := false
	for _, in := range s.eligibleInstances(r.Complexity) {
		if supportsParameters(in, r) {
			parametersSupported = true
		}
		if supports(in, r) {
			return nil
		}
	}
	if !parametersSupported {
		return apiErr(400, "invalid_request_error", "unsupported_parameter", "No instance in the compatible complexity pools supports all requested parameters.")
	}
	return apiErr(400, "invalid_request_error", "context_length_exceeded", "The request exceeds the context window of every compatible instance.")
}
func (s *Service) await(w http.ResponseWriter, r *http.Request, id string, ch chan struct{}) {
	s.mu.Lock()
	j := s.jobs[id]
	if ephemeral, ok := s.ephemeral[id]; ok {
		s.mu.Unlock()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ephemeral.status)
		_, _ = w.Write(ephemeral.body)
		return
	}
	if j != nil && (j.Status == "succeeded" || j.Status == "failed" || j.Status == "expired") {
		saved := *j
		s.mu.Unlock()
		s.respond(w, id, saved)
		return
	}
	deadline := time.Now().Add(s.cfg.RequestDeadline)
	if j != nil {
		deadline = j.Deadline
	} else if accepted := s.accepted[id]; accepted != nil {
		deadline = accepted.Deadline
	}
	s.mu.Unlock()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		return
	case <-ch:
	case <-timer.C:
		s.mu.Lock()
		j = s.jobs[id]
		if j != nil && (j.Status == "succeeded" || j.Status == "failed" || j.Status == "expired") {
			saved := *j
			s.mu.Unlock()
			s.respond(w, id, saved)
			return
		}
		queued := j != nil && j.Status == "queued" && !time.Now().Before(j.Deadline)
		s.mu.Unlock()
		if queued {
			s.expireQueued(time.Now())
			s.mu.Lock()
			j = s.jobs[id]
			if j != nil && (j.Status == "succeeded" || j.Status == "failed" || j.Status == "expired") {
				saved := *j
				s.mu.Unlock()
				s.respond(w, id, saved)
				return
			}
			s.mu.Unlock()
		}
		w.Header().Set("X-Request-ID", id)
		writeError(w, 503, "server_error", "state_unavailable", "Gateway did not persist a result before the request deadline.")
		return
	}
	s.mu.Lock()
	j = s.jobs[id]
	if j == nil {
		s.mu.Unlock()
		writeError(w, 503, "server_error", "state_unavailable", "Gateway result is unavailable.")
		return
	}
	saved := *j
	ephemeral, hasEphemeral := s.ephemeral[id]
	s.mu.Unlock()
	if hasEphemeral {
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ephemeral.status)
		_, _ = w.Write(ephemeral.body)
		return
	}
	s.respond(w, id, saved)
}
func (s *Service) respond(w http.ResponseWriter, id string, j durableJob) {
	w.Header().Set("X-Request-ID", id)
	if j.Status == "succeeded" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(j.Response)
		return
	}
	if j.Error == nil {
		writeError(w, 503, "server_error", "state_unavailable", "Gateway result is unavailable.")
		return
	}
	if j.Error.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(j.Error.RetryAfter))
	}
	writeError(w, j.Error.Status, j.Error.Type, j.Error.Code, j.Error.Message)
}

func (s *Service) writeMetrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.mu.Lock()
	defer s.mu.Unlock()
	queued := map[string]int{}
	oldest := map[string]float64{}
	results := map[string]int{"succeeded": 0, "failed": 0, "expired": 0}
	retries := 0
	waits, executions := []float64{}, []float64{}
	inputBacklog := 0
	oldestInput := 0.0
	for _, request := range s.accepted {
		inputBacklog++
		age := time.Since(request.SubmittedAt).Seconds()
		if age > oldestInput {
			oldestInput = age
		}
	}
	for _, j := range s.jobs {
		if j.Status == "queued" {
			key := j.Request.Complexity + "/" + strconv.FormatBool(j.Request.IsUrgent())
			queued[key]++
			age := time.Since(j.SubmittedAt).Seconds()
			if age > oldest[key] {
				oldest[key] = age
			}
		}
		if _, ok := results[j.Status]; ok {
			results[j.Status]++
			retries += max(0, j.Attempt-1)
			if !j.StartedAt.IsZero() {
				waits = append(waits, j.StartedAt.Sub(j.SubmittedAt).Seconds())
				if !j.FinishedAt.IsZero() {
					executions = append(executions, j.FinishedAt.Sub(j.StartedAt).Seconds())
				}
			}
		}
	}
	fmt.Fprintln(w, "# HELP gateway_queue_depth Requests waiting for an LLM slot.")
	fmt.Fprintln(w, "# TYPE gateway_queue_depth gauge")
	for _, complexity := range []string{"low", "medium", "high"} {
		for _, urgent := range []string{"false", "true"} {
			key := complexity + "/" + urgent
			fmt.Fprintf(w, "gateway_queue_depth{complexity=%q,urgent=%q} %d\n", complexity, urgent, queued[key])
			fmt.Fprintf(w, "gateway_queue_oldest_seconds{complexity=%q,urgent=%q} %.3f\n", complexity, urgent, oldest[key])
		}
	}
	fmt.Fprintln(w, "# TYPE gateway_instance_in_flight gauge")
	for _, in := range s.cfg.Instances {
		open := 0
		if !s.caller.circuit(in.ID).available(time.Now()) {
			open = 1
		}
		fmt.Fprintf(w, "gateway_instance_in_flight{instance=%q} %d\ngateway_instance_capacity{instance=%q} %d\ngateway_circuit_open{instance=%q} %d\n", in.ID, s.inFlight[in.ID], in.ID, in.MaxInFlight, in.ID, open)
	}
	fmt.Fprintln(w, "# HELP gateway_instance_expected_seconds Moving average of an instance's upstream call time; the scheduler prefers the free instance that answers sooner.")
	fmt.Fprintln(w, "# TYPE gateway_instance_expected_seconds gauge")
	for _, in := range s.cfg.Instances {
		if seconds, ok := s.expected[in.ID]; ok {
			fmt.Fprintf(w, "gateway_instance_expected_seconds{instance=%q} %.3f\n", in.ID, seconds)
		}
	}
	fmt.Fprintln(w, "# HELP gateway_route_eligible Whether an instance can serve requests at the given complexity under the configured pools.")
	fmt.Fprintln(w, "# TYPE gateway_route_eligible gauge")
	for _, complexity := range complexityLevels {
		for _, in := range s.eligibleInstances(complexity) {
			fmt.Fprintf(w, "gateway_route_eligible{complexity=%q,instance=%q,model=%q,tier=%q} 1\n", complexity, in.ID, in.Model, complexityLevels[s.instanceTier(in.ID)])
		}
	}
	fmt.Fprintln(w, "# HELP gateway_route_selections_total Requests assigned by the scheduler to an upstream instance, including retries.")
	fmt.Fprintln(w, "# TYPE gateway_route_selections_total counter")
	for _, complexity := range complexityLevels {
		for _, in := range s.eligibleInstances(complexity) {
			key := routeSelection{complexity: complexity, instance: in.ID}
			fmt.Fprintf(w, "gateway_route_selections_total{complexity=%q,instance=%q,model=%q,tier=%q} %d\n", complexity, in.ID, in.Model, complexityLevels[s.instanceTier(in.ID)], s.routeSelections[key])
		}
	}
	fmt.Fprintln(w, "# TYPE gateway_calls_total counter")
	for _, transport := range []string{"http", "grpc"} {
		for status, count := range s.transportCalls[transport] {
			fmt.Fprintf(w, "gateway_calls_total{transport=%q,status=%q} %d\n", transport, strconv.Itoa(status), count)
		}
	}
	fmt.Fprintln(w, "# TYPE gateway_requests_total counter")
	for status, count := range results {
		fmt.Fprintf(w, "gateway_requests_total{status=%q} %d\n", status, count)
	}
	fmt.Fprintf(w, "gateway_idempotent_replays_total %d\ngateway_retries_total %d\ngateway_connections_in_flight %d\ngateway_unregistered_requests %d\ngateway_input_lag_seconds %.3f\ngateway_uptime_seconds %.0f\n", s.idempotentReplays, retries, len(s.connections), inputBacklog, oldestInput, time.Since(s.started).Seconds())
	writeHistogram(w, "gateway_queue_wait_seconds", waits)
	writeHistogram(w, "gateway_execution_seconds", executions)
}

func writeHistogram(w http.ResponseWriter, name string, samples []float64) {
	buckets := []float64{1, 5, 10, 30, 60, 120, 180}
	sum := 0.0
	for _, sample := range samples {
		sum += sample
	}
	for _, bound := range buckets {
		count := 0
		for _, sample := range samples {
			if sample <= bound {
				count++
			}
		}
		fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, strconv.FormatFloat(bound, 'f', -1, 64), count)
	}
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n%s_sum %.3f\n%s_count %d\n", name, len(samples), name, sum, name, len(samples))
}
func writeError(w http.ResponseWriter, status int, typ, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{Error: APIError{Message: message, Type: typ, Param: nil, Code: code}})
}
func writeStatusError(w http.ResponseWriter, err error) {
	var e statusError
	if errors.As(err, &e) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(e.status)
		_ = json.NewEncoder(w).Encode(e.body)
		return
	}
	writeError(w, 400, "invalid_request_error", "invalid_request", err.Error())
}
func errorBody(typ, code, msg string) []byte {
	b, _ := json.Marshal(ErrorEnvelope{Error: APIError{Message: msg, Type: typ, Code: code, Param: nil}})
	return b
}
func identity(client, key string) string { return client + "\x00" + key }
func isPending(status string) bool {
	return status == "queued" || status == "running" || status == "expiring"
}
func expiredForRetention(j *durableJob, now time.Time, retention time.Duration) bool {
	if j == nil || isPending(j.Status) || j.SlotHeld {
		return false
	}
	completedAt := j.FinishedAt
	if completedAt.IsZero() {
		completedAt = j.SubmittedAt
	}
	return !completedAt.IsZero() && now.Sub(completedAt) >= retention
}
func stateKey(j *durableJob) []byte {
	if j.IdempotencyKey == "" {
		return []byte(j.ID)
	}
	h := sha256.Sum256([]byte(identity(j.ClientID, j.IdempotencyKey)))
	return []byte(hex.EncodeToString(h[:]))
}
func stableJobID(client, key string) string {
	h := sha256.Sum256([]byte(identity(client, key)))
	return hex.EncodeToString(h[:16])
}
func newID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
func ioReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func (s *Service) Handler() http.Handler { return http.HandlerFunc(s.handler) }
