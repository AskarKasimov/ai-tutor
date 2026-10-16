package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Completion struct {
	status     int
	body       []byte
	retryAfter int
}

// Complete submits a normalized request and awaits its durable result.
func (s *Service) Complete(ctx context.Context, req Request, key, transport string, admitted bool, onID func(string)) (Completion, string, error) {
	result, id, err := s.complete(ctx, req, key, admitted, onID)
	statusCode := result.status
	var se statusError
	if errors.As(err, &se) {
		statusCode = se.status
	} else if errors.Is(err, context.Canceled) {
		statusCode = 499
	} else if errors.Is(err, context.DeadlineExceeded) {
		statusCode = 504
	} else if err != nil {
		statusCode = 503
	}
	s.recordCall(transport, statusCode)
	return result, id, err
}
func (s *Service) complete(ctx context.Context, req Request, key string, admitted bool, onID func(string)) (Completion, string, error) {
	announced := false
	announce := func(id string) {
		if !announced && onID != nil {
			onID(id)
			announced = true
		}
	}
	attemptID := newID()
	defer func() { announce(attemptID) }()
	key = strings.TrimSpace(key)
	if err := ctx.Err(); err != nil {
		return Completion{}, attemptID, err
	}
	if !admitted {
		select {
		case s.connections <- struct{}{}:
			defer func() { <-s.connections }()
		default:
			return Completion{}, attemptID, apiErr(429, "rate_limit_error", "connection_limit", "Too many concurrent requests.")
		}
	}
	s.mu.Lock()
	ready := s.ready && !s.stopping
	s.mu.Unlock()
	if !ready {
		return Completion{}, attemptID, apiErr(503, "server_error", "gateway_not_ready", "Gateway is not ready.")
	}
	if int64(len(canonicalRequestJSON(req))) > s.cfg.MaxBodyBytes {
		return Completion{}, attemptID, apiErr(413, "invalid_request_error", "request_too_large", "Request body is too large.")
	}
	if err := s.validatePool(req); err != nil {
		return Completion{}, attemptID, err
	}
	clientID := "internal"
	hash := req.CanonicalHash()
	created := time.Now()
	deadline := created.Add(s.cfg.RequestDeadline)
	id := newID()
	if key != "" {
		id = stableJobID(clientID, key)
	}
	payload := durableRequest{Version: 1, ID: id, ClientID: clientID, IdempotencyKey: key, Hash: hash, Request: req, SubmittedAt: created, Deadline: deadline}
	queuedBytes := int64(len(mustJSON(payload)))
	s.mu.Lock()
	if key != "" {
		index := identity(clientID, key)
		if oldID, ok := s.keys[index]; ok {
			if old := s.jobs[oldID]; old != nil && !isPending(old.Status) && !old.SlotHeld && time.Since(old.SubmittedAt) >= s.cfg.Retention {
				delete(s.jobs, oldID)
				delete(s.keys, index)
			}
		}
		if oldID, ok := s.keys[index]; ok {
			s.idempotentReplays++
			old := s.jobs[oldID]
			var oldReq Request
			oldHash := ""
			if old != nil {
				oldReq = old.Request
				oldHash = old.Hash
			} else if accepted := s.accepted[oldID]; accepted != nil {
				oldReq = accepted.Request
				oldHash = accepted.Hash
			}
			matches := oldReq.CanonicalHash() == hash
			if !matches && len(oldReq.Parameters) == 0 && oldHash != "" {
				for _, candidate := range req.legacyHashes() {
					if candidate == oldHash {
						matches = true
						break
					}
				}
			}
			if !matches {
				s.mu.Unlock()
				announce(oldID)
				return Completion{}, oldID, apiErr(409, "idempotency_error", "idempotency_conflict", "Idempotency-Key was already used with a different request.")
			}
			id = oldID
			if old != nil && !isPending(old.Status) {
				result := completionFromJob(*old)
				s.mu.Unlock()
				announce(id)
				return result, id, nil
			}
			if s.cancelled[id] {
				ch := s.waiters[id]
				if ch == nil {
					ch = make(chan struct{})
					s.waiters[id] = ch
				}
				s.mu.Unlock()
				announce(id)
				result, err := s.awaitCompletion(ctx, id, ch)
				return result, id, err
			}
			ch := s.waiters[id]
			if ch == nil {
				ch = make(chan struct{})
				s.waiters[id] = ch
			}
			s.attachWaiter(id)
			s.mu.Unlock()
			announce(id)
			defer s.detachWaiter(id)
			result, err := s.awaitCompletion(ctx, id, ch)
			return result, id, err
		}
	}
	pending := 0
	for _, j := range s.jobs {
		if isPending(j.Status) {
			pending++
		}
	}
	if pending+s.reserved >= s.cfg.MaxPending || s.bytes+s.reservedBytes+queuedBytes > s.cfg.MaxPendingBytes {
		s.mu.Unlock()
		announce(id)
		return Completion{}, id, apiErr(429, "rate_limit_error", "gateway_overloaded", "Gateway queue capacity has been reached.")
	}
	ch := make(chan struct{})
	s.waiters[id] = ch
	s.accepted[id] = &payload
	if key != "" {
		s.keys[identity(clientID, key)] = id
	}
	s.reserved++
	s.reservedBytes += queuedBytes
	s.attachWaiter(id)
	s.mu.Unlock()
	announce(id)
	defer s.detachWaiter(id)
	raw, _ := json.Marshal(payload)
	submitCtx, stop := context.WithDeadline(ctx, deadline)
	produced := s.submitter.ProduceSync(submitCtx, &kgo.Record{Topic: s.cfg.RequestsTopic, Key: []byte(id), Value: raw})
	stop()
	if err := produced.FirstErr(); err != nil {
		if ctx.Err() != nil {
			return Completion{}, id, ctx.Err()
		}
		go s.retryPublish(payload, raw, queuedBytes)
		return Completion{}, id, apiErr(503, "server_error", "queue_unavailable", "Gateway could not durably accept the request.")
	}
	result, err := s.awaitCompletion(ctx, id, ch)
	return result, id, err
}

func completionFromJob(j durableJob) Completion {
	if j.Status == "succeeded" {
		return Completion{status: 200, body: append([]byte(nil), j.Response...)}
	}
	if j.Error != nil {
		return Completion{status: j.Error.Status, body: errorBody(j.Error.Type, j.Error.Code, j.Error.Message), retryAfter: j.Error.RetryAfter}
	}
	return Completion{status: 503, body: errorBody("server_error", "state_unavailable", "Gateway result is unavailable.")}
}
func (s *Service) awaitCompletion(ctx context.Context, id string, ch chan struct{}) (Completion, error) {
	s.mu.Lock()
	j := s.jobs[id]
	ep, hasEP := s.ephemeral[id]
	deadline := time.Now().Add(s.cfg.RequestDeadline)
	if j != nil {
		deadline = j.Deadline
	} else if accepted := s.accepted[id]; accepted != nil {
		deadline = accepted.Deadline
	}
	if hasEP {
		s.mu.Unlock()
		return Completion{status: ep.status, body: ep.body}, nil
	}
	if j != nil && !isPending(j.Status) {
		result := completionFromJob(*j)
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Completion{}, ctx.Err()
	case <-ch:
	case <-timer.C:
	}
	s.mu.Lock()
	j = s.jobs[id]
	ep, hasEP = s.ephemeral[id]
	if hasEP {
		s.mu.Unlock()
		return Completion{status: ep.status, body: ep.body}, nil
	}
	if j != nil && !isPending(j.Status) {
		result := completionFromJob(*j)
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	return Completion{}, apiErr(503, "server_error", "state_unavailable", "Gateway did not persist a result before the request deadline.")
}
func (s *Service) attachWaiter(id string) {
	if s.activeWaiters == nil {
		s.activeWaiters = map[string]int{}
	}
	s.activeWaiters[id]++
}
func (s *Service) detachWaiter(id string) {
	s.mu.Lock()
	if s.activeWaiters[id] > 0 {
		s.activeWaiters[id]--
	}
	last := s.activeWaiters[id] == 0
	if last {
		delete(s.activeWaiters, id)
	}
	j := s.jobs[id]
	accepted := s.accepted[id]
	if !last || (j == nil && accepted == nil) || j != nil && !isPending(j.Status) {
		s.mu.Unlock()
		return
	}
	if s.cancelled == nil {
		s.cancelled = map[string]bool{}
	}
	s.cancelled[id] = true
	if cancel := s.jobCancels[id]; cancel != nil {
		cancel()
	}
	s.mu.Unlock()
	go s.persistCancellation(id, "request_canceled")
}
func (s *Service) persistCancellation(id, code string) {
	s.mu.Lock()
	j := s.jobs[id]
	accepted := s.accepted[id]
	if j == nil && accepted != nil {
		j = &durableJob{Version: 1, ID: accepted.ID, ClientID: accepted.ClientID, IdempotencyKey: accepted.IdempotencyKey, Hash: accepted.Hash, Request: accepted.Request, SubmittedAt: accepted.SubmittedAt, Deadline: accepted.Deadline}
	}
	if j == nil {
		s.mu.Unlock()
		return
	}
	copy := *j
	s.mu.Unlock()
	copy.Status = "failed"
	copy.SlotHeld = false
	copy.FinishedAt = time.Now()
	copy.Error = &storedError{Status: 409, Type: "request_cancelled", Code: code, Message: "The request was canceled because no clients are waiting."}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.save(ctx, &copy); err != nil {
		if errors.Is(err, errCanceledJob) {
			return
		}
		s.markStateUncertain()
		s.notifyEphemeral(id, submitResult{503, errorBody("server_error", "state_unavailable", "Gateway could not durably save cancellation.")})
		return
	}
	s.mu.Lock()
	if ch := s.waiters[id]; ch != nil {
		close(ch)
		delete(s.waiters, id)
	}
	s.mu.Unlock()
}

var errCanceledJob = errors.New("job canceled")

func (s *Service) isCanceled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled[id]
}
func (s *Service) Ready() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.ready && !s.stopping }

func (s *Service) markStateUncertain() {
	s.mu.Lock()
	s.ready = false
	s.brokerAvailable = false
	s.stateUncertain = true
	s.mu.Unlock()
}

func canonicalRequestJSON(req Request) []byte {
	fields := map[string]any{"messages": req.Messages, "complexity": req.Complexity, "urgent": req.IsUrgent()}
	for name, raw := range req.Parameters {
		fields[name] = raw
	}
	b, _ := json.Marshal(fields)
	return b
}

func (s *Service) recordCall(transport string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transportCalls == nil {
		s.transportCalls = map[string]map[int]uint64{}
	}
	if s.transportCalls[transport] == nil {
		s.transportCalls[transport] = map[int]uint64{}
	}
	s.transportCalls[transport][status]++
}

func (s *Service) correctCall(transport string, from, to int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transportCalls[transport][from] > 0 {
		s.transportCalls[transport][from]--
	}
	s.transportCalls[transport][to]++
}
