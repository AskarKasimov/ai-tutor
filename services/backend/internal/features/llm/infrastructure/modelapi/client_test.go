package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolBoundsRequestsAndCancelsQueue(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "{\"remove\":[]}"}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	client := New(server.Client(), []string{server.URL}, "test", "", time.Second)
	done := make(chan error, 1)
	go func() { _, err := client.Complete(context.Background(), "system", "input"); done <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := client.Complete(ctx, "system", "input"); err == nil {
		t.Fatal("queued call ignored cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("more than one in-flight request per endpoint")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestIncompleteModelResponseRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()
	if _, err := New(server.Client(), []string{server.URL}, "test", "", time.Second).Complete(context.Background(), "s", "i"); err == nil {
		t.Fatal("truncated response accepted")
	}
}
