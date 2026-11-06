// Package execution persists task transitions before scheduling model calls.
package execution

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Job struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Hash      string    `json:"hash"`
	Key       string    `json:"key,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	State     string    `json:"state"`
	Instance  string    `json:"instance,omitempty"`
	Held      bool      `json:"held,omitempty"`
	Code      int32     `json:"code,omitempty"`
	Submitted time.Time `json:"submitted"`
	Finished  time.Time `json:"finished,omitempty"`
}
type Engine struct {
	failed    chan struct{}
	failOnce  sync.Once
	workers   sync.WaitGroup
	mu        sync.Mutex
	store     journal.Store
	blobs     *journal.Blobs
	queues    map[string]*scheduler.Queue
	jobs      map[string]*Job
	keys      map[string]string
	done      map[string]chan struct{}
	abort     func(string) bool
	retention time.Duration
	ready     bool
	logger    *slog.Logger
	selected  map[string]uint64
	completed map[string]uint64
	durations map[string]*histogram
}

func New(store journal.Store, blobs *journal.Blobs, queues map[string]*scheduler.Queue, retention time.Duration, abort func(string) bool, logger *slog.Logger) *Engine {
	return &Engine{failed: make(chan struct{}), store: store, blobs: blobs, queues: queues, retention: retention, abort: abort, logger: logger, jobs: map[string]*Job{}, keys: map[string]string{}, done: map[string]chan struct{}{}, selected: map[string]uint64{}, completed: map[string]uint64{}, durations: map[string]*histogram{}}
}
func (e *Engine) save(j *Job) error {
	return e.saveContext(context.Background(), j)
}
func (e *Engine) saveContext(parent context.Context, j *Job) error {
	payload, err := json.Marshal(j)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	if err = e.store.Append(ctx, journal.Record{Operation: j.Operation, ID: j.ID, Value: payload}); err != nil {
		e.failLocked()
		return err
	}
	return nil
}
func (e *Engine) Restore(ctx context.Context) error {
	records, err := e.store.Replay(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		var j Job
		if err = json.Unmarshal(record.Value, &j); err != nil {
			return fmt.Errorf("invalid voice task state: %w", err)
		}
		if j.ID != record.ID || j.Operation != record.Operation || e.queues[j.Operation] == nil || !validID(j.ID) {
			return errors.New("invalid voice task identity")
		}
		switch j.State {
		case "queued", "running", "failed", "succeeded":
		default:
			return errors.New("invalid voice task status")
		}
		changed := false
		if j.State == "running" {
			changed = true
			j.State = "failed"
			j.Code = int32(codes.Aborted)
			j.Held = !e.abort(j.Instance)
			j.Finished = time.Now()
		}
		if j.State == "queued" {
			changed = true
			j.State = "failed"
			j.Code = int32(codes.Canceled)
			j.Finished = time.Now()
		}
		if j.Held && e.abort(j.Instance) {
			changed = true
			j.Held = false
		}
		if j.Held {
			if err = e.queues[j.Operation].RestoreHeld(j.Instance, 1); err != nil {
				return err
			}
		}
		if changed {
			if err = e.saveContext(ctx, &j); err != nil {
				return err
			}
		}
		copy := j
		e.jobs[j.ID] = &copy
		if j.Key != "" {
			e.keys[j.Operation+"/"+j.Key] = j.ID
		}
	}
	names, err := e.blobs.Names()
	if err != nil {
		return err
	}
	for _, name := range names {
		j := e.jobs[name[:32]]
		if j == nil || name[33:] == "in" || j.State != "succeeded" {
			if err = e.blobs.Remove(name); err != nil {
				return err
			}
		}
	}
	e.ready = true
	return e.cleanLocked(ctx, time.Now())
}
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func (e *Engine) Ready() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.ready }
func (e *Engine) Stop()       { e.mu.Lock(); e.ready = false; e.mu.Unlock() }
func (e *Engine) resultLocked(j *Job) scheduler.Result {
	if j == nil {
		return scheduler.Result{Err: status.Error(codes.NotFound, "voice result expired")}
	}
	if j.State != "succeeded" {
		return scheduler.Result{Err: status.Error(codes.Code(j.Code), "voice task failed")}
	}
	data, err := e.blobs.Get(j.ID+".out", 65<<20)
	if err != nil {
		return scheduler.Result{Err: status.Error(codes.DataLoss, "voice result unavailable")}
	}
	return scheduler.Result{Value: data}
}

// Submit stores the request before admitting it to the operation's queue. call
// returns a protobuf response encoded as bytes; provider failures are safe errors.
func (e *Engine) Submit(ctx context.Context, op, profile, key string, input []byte, announce func(string), call func(context.Context, string) scheduler.Result) (scheduler.Result, string) {
	hash := sha256.Sum256(input)
	hashString := hex.EncodeToString(hash[:])
	keyHash := ""
	if key != "" {
		sum := sha256.Sum256([]byte(key))
		keyHash = hex.EncodeToString(sum[:])
	}
	e.mu.Lock()
	if !e.ready {
		e.mu.Unlock()
		return scheduler.Result{Err: status.Error(codes.Unavailable, "voice gateway not ready")}, ""
	}
	if id, ok := e.keys[op+"/"+keyHash]; keyHash != "" && ok {
		j := e.jobs[id]
		if j.Hash != hashString {
			e.mu.Unlock()
			return scheduler.Result{Err: status.Error(codes.AlreadyExists, "idempotency key conflict")}, id
		}
		ch := e.done[id]
		if ch == nil {
			r := e.resultLocked(j)
			e.mu.Unlock()
			announce(id)
			return r, id
		}
		e.mu.Unlock()
		announce(id)
		select {
		case <-ch:
			e.mu.Lock()
			r := e.resultLocked(e.jobs[id])
			e.mu.Unlock()
			return r, id
		case <-ctx.Done():
			return scheduler.Result{Err: ctx.Err()}, id
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		e.mu.Unlock()
		return scheduler.Result{Err: err}, ""
	}
	id := hex.EncodeToString(idBytes)
	j := &Job{ID: id, Operation: op, Profile: profile, Hash: hashString, Key: keyHash, State: "queued", Submitted: time.Now()}
	if err := e.blobs.Put(id+".in", input); err != nil {
		e.mu.Unlock()
		return scheduler.Result{Err: status.Error(codes.ResourceExhausted, "voice payload storage unavailable")}, id
	}
	if err := e.save(j); err != nil {
		e.mu.Unlock()
		return scheduler.Result{Err: status.Error(codes.Unavailable, "voice queue unavailable")}, id
	}
	e.jobs[id] = j
	e.done[id] = make(chan struct{})
	if keyHash != "" {
		e.keys[op+"/"+keyHash] = id
	}
	e.mu.Unlock()
	announce(id)
	result := e.queues[op].Submit(ctx, profile, func(callCtx context.Context, instance string) scheduler.Result {
		e.mu.Lock()
		if !e.ready {
			e.mu.Unlock()
			return scheduler.Result{Err: status.Error(codes.Unavailable, "voice gateway not ready")}
		}
		e.workers.Add(1)
		defer e.workers.Done()
		j.State = "running"
		j.Instance = instance
		j.Held = true
		if err := e.save(j); err != nil {
			e.mu.Unlock()
			return scheduler.Result{Err: status.Error(codes.Unavailable, "voice state unavailable"), HoldSlot: true}
		}
		e.selected[op+"/"+instance]++
		e.observe(op, "queue_wait", time.Since(j.Submitted).Seconds())
		e.mu.Unlock()
		started := time.Now()
		r := call(callCtx, instance)
		elapsed := time.Since(started)
		// Disconnects and transport failures do not prove model execution stopped.
		if e.abort(instance) {
			r.HoldSlot = false
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		j.Finished = time.Now()
		j.Held = r.HoldSlot
		j.Code = int32(errorCode(r.Err))
		if r.Err == nil {
			bytes, ok := r.Value.([]byte)
			if !ok {
				r.Err = status.Error(codes.Internal, "invalid voice result")
			} else if err := e.blobs.Put(id+".out", bytes); err != nil {
				r.Err = status.Error(codes.Unavailable, "voice result storage unavailable")
			}
		}
		if r.Err == nil {
			j.State = "succeeded"
			j.Code = 0
		} else {
			j.State = "failed"
			j.Code = int32(errorCode(r.Err))
		}
		if err := e.save(j); err != nil {
			r.Err = status.Error(codes.Unavailable, "voice result not committed")
			r.HoldSlot = true
		} else {
			e.finishLocked(j)
			_ = e.blobs.Remove(id + ".in")
		}
		e.observe(op, "upstream", elapsed.Seconds())
		e.completed[op+"/"+errorCode(r.Err).String()]++
		e.logger.Log(context.Background(), logLevel(r.Err), "voice upstream call", slog.String("gateway_request_id", id), slog.String("operation", op), slog.String("instance", instance), slog.String("code", errorCode(r.Err).String()), slog.Duration("duration", elapsed), slog.Bool("slot_held", j.Held))
		return r
	})
	e.mu.Lock()
	if j.State == "queued" {
		j.State = "failed"
		j.Code = int32(errorCode(result.Err))
		j.Finished = time.Now()
		if err := e.save(j); err == nil {
			e.finishLocked(j)
			_ = e.blobs.Remove(id + ".in")
		}
	}
	e.mu.Unlock()
	return result, id
}
func logLevel(err error) slog.Level {
	if err == nil {
		return slog.LevelInfo
	}
	return slog.LevelWarn
}
func errorCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}
	if errors.Is(err, scheduler.ErrQueueFull) {
		return codes.ResourceExhausted
	}
	if errors.Is(err, scheduler.ErrNoCompatibleInstance) {
		return codes.Unavailable
	}
	if status.Code(err) == codes.Unknown {
		return codes.Unavailable
	}
	return status.Code(err)
}
func (e *Engine) finishLocked(j *Job) {
	if ch := e.done[j.ID]; ch != nil {
		close(ch)
		delete(e.done, j.ID)
	}
}
func (e *Engine) cleanLocked(parent context.Context, now time.Time) error {
	for id, j := range e.jobs {
		if j.Held || j.Finished.IsZero() || now.Sub(j.Finished) < e.retention {
			continue
		}
		ctx, cancel := context.WithTimeout(parent, 6*time.Second)
		err := e.store.Append(ctx, journal.Record{Operation: j.Operation, ID: id})
		cancel()
		if err != nil {
			e.failLocked()
			return err
		}
		if err = e.blobs.Remove(id + ".out"); err != nil {
			return err
		}
		if err = e.blobs.Remove(id + ".in"); err != nil {
			return err
		}
		delete(e.jobs, id)
		if j.Key != "" {
			delete(e.keys, j.Operation+"/"+j.Key)
		}
	}
	return nil
}
func (e *Engine) Maintain(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.mu.Lock()
			err := e.cleanLocked(ctx, now)
			e.mu.Unlock()
			if err != nil {
				e.logger.Error("voice cleanup failed", slog.String("reason", "storage unavailable"))
			}
		}
	}
}

func (e *Engine) Wait() { e.workers.Wait() }

// ReleaseHeld commits removal of terminal reservations before allowing new calls.
// It never releases a currently running job's slot.
func (e *Engine) ReleaseHeld(instance string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return errors.New("gateway not ready")
	}
	count := 0
	op := ""
	for _, j := range e.jobs {
		if j.Instance == instance && j.Held && j.State != "running" {
			copy := *j
			copy.Held = false
			if err := e.save(&copy); err != nil {
				return err
			}
			*j = copy
			count++
			op = j.Operation
		}
	}
	if count == 0 {
		return errors.New("no held slots for instance")
	}
	return e.queues[op].ReleaseHeld(instance, count)
}

func (e *Engine) failLocked()             { e.ready = false; e.failOnce.Do(func() { close(e.failed) }) }
func (e *Engine) Failed() <-chan struct{} { return e.failed }
