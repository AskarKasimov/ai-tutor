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
	ready             bool
	brokerAvailable   bool
	stateUncertain    bool
	stopping          bool
	started           time.Time
	idempotentReplays uint64
}

func NewService(cfg Config) (*Service, error) {
	cfg.defaults()
	s := &Service{cfg: cfg, jobs: map[string]*durableJob{}, keys: map[string]string{}, accepted: map[string]*durableRequest{}, stateIDs: map[string]string{}, waiters: map[string]chan struct{}{}, activeWaiters: map[string]int{}, cancelled: map[string]bool{}, jobCancels: map[string]context.CancelFunc{}, ephemeral: map[string]submitResult{}, inFlight: map[string]int{}, connections: make(chan struct{}, cfg.MaxConnections), wake: make(chan struct{}, 1), roundRobin: map[string]int{}, started: time.Now()}
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
	s.ingest, err = kgo.NewGroupTransactSession(kgo.SeedBrokers(brokers...), kgo.TransactionalID(cfg.TransactionalID+"-ingest"), kgo.ConsumerGroup(cfg.ConsumerGroup), kgo.ConsumeTopics(cfg.RequestsTopic), kgo.ConsumeResetOffset(kgo.NoResetOffset()), kgo.FetchIsolationLevel(kgo.ReadCommitted()), kgo.DisableAutoCommit(), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.ProducerBatchMaxBytes(40<<20))
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
		count := 0
		for it := fs.RecordIter(); !it.Done(); {
			r := it.Next()
			count++
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
		if count == 0 {
			s.mu.Lock()
			var recoveries []*durableJob
			for _, j := range s.jobs {
				if j.Error != nil && j.Error.Code == "execution_interrupted" || j.Status == "queued" {
					copy := *j
					if copy.Status == "queued" {
						copy.Status = "failed"
						copy.Error = &storedError{Status: 409, Type: "request_cancelled", Code: "client_disconnected", Message: "No client remains connected after gateway restart."}
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

func (s *Service) install(j *durableJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.Status == "succeeded" || j.Status == "failed" || j.Status == "expired" {
		if time.Since(j.SubmittedAt) > s.cfg.Retention {
			return
		}
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
		var duplicateState *durableJob
		if request.IdempotencyKey != "" {
			if existingID, ok := s.keys[identity(request.ClientID, request.IdempotencyKey)]; ok && existingID != request.ID {
				duplicate = true
				if existing := s.jobs[existingID]; existing != nil {
					copy := *existing
					duplicateState = &copy
				}
			}
		}
		if existing, ok := s.jobs[request.ID]; ok {
			duplicate = true
			copy := *existing
			duplicateState = &copy
		}
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
			raw, _ := json.Marshal(job)
			if got := s.ingest.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: stateKey(job), Value: raw}); got.FirstErr() != nil {
				_, _ = s.ingest.End(context.Background(), kgo.TryAbort)
				s.stateMu.Unlock()
				time.Sleep(time.Second)
				continue
			}
		} else if duplicateState != nil {
			raw, _ := json.Marshal(duplicateState)
			if got := s.ingest.ProduceSync(ctx, &kgo.Record{Topic: s.cfg.StateTopic, Key: stateKey(duplicateState), Value: raw}); got.FirstErr() != nil {
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
			s.signal()
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
				if j.ProbeReserved {
					s.caller.circuit(in.ID).releaseReservation()
				}
				s.releaseSlot(j)
				break
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
		for _, job := range pending {
			if time.Now().Before(job.NotBefore) {
				continue
			}
			if (pass == 0) != job.Request.IsUrgent() {
				continue
			}
			pool := s.cfg.Pools[job.Request.Complexity]
			var candidates []InstanceConfig
			for _, id := range pool {
				in := s.instance(id)
				if in.ID == "" || s.inFlight[id] >= in.MaxInFlight || !supports(in, job.Request) || !s.caller.circuit(id).available(time.Now()) {
					continue
				}
				candidates = append(candidates, in)
			}
			if len(candidates) == 0 {
				continue
			}
			sort.SliceStable(candidates, func(a, b int) bool {
				ra := float64(s.inFlight[candidates[a].ID]) / float64(candidates[a].MaxInFlight)
				rb := float64(s.inFlight[candidates[b].ID]) / float64(candidates[b].MaxInFlight)
				return ra < rb
			})
			best := float64(s.inFlight[candidates[0].ID]) / float64(candidates[0].MaxInFlight)
			ties := make([]InstanceConfig, 0, len(candidates))
			for _, candidate := range candidates {
				if float64(s.inFlight[candidate.ID])/float64(candidate.MaxInFlight) == best {
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
func supports(in InstanceConfig, r Request) bool {
	if !supportsParameters(in, r) {
		return false
	}
	chars := 0
	for _, m := range r.Messages {
		chars += len([]byte(m.Content))/2 + 8
	}
	tokens := chars
	max := in.ContextWindow
	if raw, ok := r.Parameters["max_tokens"]; ok {
		var n int
		_ = json.Unmarshal(raw, &n)
		tokens += n
	}
	if raw, ok := r.Parameters["max_completion_tokens"]; ok {
		var n int
		_ = json.Unmarshal(raw, &n)
		tokens += n
	}
	return tokens <= max
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
	result, failure := s.caller.call(callCtx, in, j.Request, j.ID, j.ProbeReserved)
	cancel()
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
		uncertain := errors.Is(failure.Err, context.DeadlineExceeded) || (failure.Code == "upstream_transport_error" && !failure.BeforeSend)
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
		if !isPending(j.Status) && !j.SlotHeld && now.Sub(j.SubmittedAt) >= s.cfg.Retention {
			copy := *j
			expired = append(expired, &copy)
		}
	}
	s.mu.Unlock()
	for _, j := range expired {
		s.stateMu.Lock()
		s.mu.Lock()
		current := s.jobs[j.ID]
		stillExpired := current != nil && current.ID == j.ID && !isPending(current.Status) && !current.SlotHeld && now.Sub(current.SubmittedAt) >= s.cfg.Retention
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
		if current := s.jobs[j.ID]; current != nil && !isPending(current.Status) && !current.SlotHeld && now.Sub(current.SubmittedAt) >= s.cfg.Retention {
			delete(s.jobs, j.ID)
			delete(s.stateIDs, string(stateKey(current)))
			if current.IdempotencyKey != "" {
				key := identity(current.ClientID, current.IdempotencyKey)
				if s.keys[key] == current.ID {
					delete(s.keys, key)
				}
			}
			if current.SlotHeld && current.Instance != "" && s.inFlight[current.Instance] > 0 {
				s.inFlight[current.Instance]--
			}
		}
		s.mu.Unlock()
		s.stateMu.Unlock()
	}
}
func (s *Service) finish(j *durableJob, result submitResult, state string) {
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
	} else {
		s.markStateUncertain()
		s.notifyEphemeral(j.ID, submitResult{503, errorBody("server_error", "state_unavailable", "Gateway could not durably save the request state.")})
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
	for _, id := range s.cfg.Pools[r.Complexity] {
		in := s.instance(id)
		if supportsParameters(in, r) {
			parametersSupported = true
		}
		if supports(in, r) {
			return nil
		}
	}
	if !parametersSupported {
		return apiErr(400, "invalid_request_error", "unsupported_parameter", "No instance in the selected complexity pool supports all requested parameters.")
	}
	return apiErr(400, "invalid_request_error", "context_length_exceeded", "The request exceeds the context window configured for this complexity pool.")
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
		s.mu.Unlock()
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
func isPending(status string) bool       { return status == "queued" || status == "running" }
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
