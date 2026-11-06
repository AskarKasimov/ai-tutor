package execution

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setup(t *testing.T, store *testutil.Memory, dir string) *Engine {
	t.Helper()
	blobs, err := journal.OpenBlobs(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(blobs.Close)
	q, err := scheduler.New([]scheduler.Instance{{ID: "one", Profiles: []string{"voice"}, Slots: 1}}, 4)
	if err != nil {
		t.Fatal(err)
	}
	tts, _ := scheduler.New([]scheduler.Instance{{ID: "tts", Profiles: []string{"voice"}, Slots: 1}}, 4)
	e := New(store, blobs, map[string]*scheduler.Queue{"stt": q, "tts": tts}, time.Hour, func(string) bool { return false }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = e.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestDurableResultAndIdempotentReplay(t *testing.T) {
	store := &testutil.Memory{}
	dir := t.TempDir()
	e := setup(t, store, dir)
	calls := 0
	call := func(context.Context, string) scheduler.Result {
		calls++
		return scheduler.Result{Value: []byte("private result")}
	}
	r, id := e.Submit(context.Background(), "stt", "", "same", []byte("private request"), func(string) {}, call)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	r, replayID := e.Submit(context.Background(), "stt", "", "same", []byte("private request"), func(string) {}, call)
	if r.Err != nil || id != replayID || calls != 1 {
		t.Fatalf("replay: %+v calls %d", r, calls)
	}
	r, _ = e.Submit(context.Background(), "stt", "", "same", []byte("changed"), func(string) {}, call)
	if status.Code(r.Err) != codes.AlreadyExists {
		t.Fatal(r.Err)
	}
	recorder := httptest.NewRecorder()
	e.Metrics(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(recorder.Body.String(), "private") {
		t.Fatal("content leaked into metrics")
	}
	e.blobs.Close()
	restarted := setup(t, store, dir)
	r, replayID = restarted.Submit(context.Background(), "stt", "", "same", []byte("private request"), func(string) {}, call)
	if r.Err != nil || id != replayID || calls != 1 {
		t.Fatal("restart lost result")
	}
}
func TestFailedPersistenceDoesNotCallModel(t *testing.T) {
	store := &testutil.Memory{}
	e := setup(t, store, t.TempDir())
	store.Mu.Lock()
	store.Fail = true
	store.Mu.Unlock()
	called := false
	r, _ := e.Submit(context.Background(), "stt", "", "", []byte("audio"), func(string) {}, func(context.Context, string) scheduler.Result { called = true; return scheduler.Result{} })
	if r.Err == nil || called || e.Ready() {
		t.Fatalf("failed journal admitted call: %+v", r)
	}
}
func TestRecoveryKeepsUncertainSlot(t *testing.T) {
	store := &testutil.Memory{}
	dir := t.TempDir()
	e := setup(t, store, dir)
	r, _ := e.Submit(context.Background(), "stt", "", "", []byte("audio"), func(string) {}, func(context.Context, string) scheduler.Result {
		return scheduler.Result{Err: context.DeadlineExceeded, HoldSlot: true}
	})
	if !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Fatal(r.Err)
	}
	e.blobs.Close()
	restarted := setup(t, store, dir)
	if restarted.queues["stt"].Stats().Instances[0].Busy != 1 {
		t.Fatal("uncertain slot lost on restart")
	}
}

func TestOperatorReleaseIsDurable(t *testing.T) {
	store := &testutil.Memory{}
	dir := t.TempDir()
	e := setup(t, store, dir)
	e.Submit(context.Background(), "stt", "", "", []byte("audio"), func(string) {}, func(context.Context, string) scheduler.Result {
		return scheduler.Result{Err: context.DeadlineExceeded, HoldSlot: true}
	})
	if err := e.ReleaseHeld("one"); err != nil {
		t.Fatal(err)
	}
	if e.queues["stt"].Stats().Instances[0].Busy != 0 {
		t.Fatal("slot still held")
	}
	e.blobs.Close()
	restored := setup(t, store, dir)
	if restored.queues["stt"].Stats().Instances[0].Busy != 0 {
		t.Fatal("release was not committed")
	}
}
