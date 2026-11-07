package gateway

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// openWithOlderCalls returns tickets of n calls reserved while the breaker was
// closed, opens the breaker by three other failures (the pause already ended)
// and takes the half-open probe.
func openWithOlderCalls(t *testing.T, c *Circuit, n int) (older []circuitTicket, probe circuitTicket) {
	t.Helper()
	now := time.Now()
	for range n {
		ticket, ok := c.reserve(now)
		if !ok || ticket.probe {
			t.Fatalf("closed breaker: reserve = (%+v, %v)", ticket, ok)
		}
		older = append(older, ticket)
	}
	past := now.Add(-time.Minute)
	for range 3 {
		ticket, ok := c.reserve(past)
		if !ok {
			t.Fatal("closed breaker rejected a call")
		}
		c.release(false, ticket, past)
	}
	probe, ok := c.reserve(now)
	if !ok || !probe.probe {
		t.Fatalf("reserve = (%+v, %v), want the half-open probe", probe, ok)
	}
	return older, probe
}

func TestLateSuccessOfACallBeforeOpeningDoesNotLetRequestsThroughDuringTheProbe(t *testing.T) {
	var c Circuit
	older, _ := openWithOlderCalls(t, &c, 1)
	c.release(true, older[0], time.Now())
	for i := range 5 {
		if _, ok := c.reserve(time.Now()); ok {
			t.Fatalf("reserve %d granted before the outcome of the probe", i)
		}
	}
}

func TestLateFailureOfACallBeforeOpeningDoesNotExtendThePause(t *testing.T) {
	var c Circuit
	older, _ := openWithOlderCalls(t, &c, 1)
	c.mu.Lock()
	failures, openUntil := c.failures, c.openUntil
	c.mu.Unlock()
	c.release(false, older[0], time.Now().Add(10*time.Second))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures != failures || !c.openUntil.Equal(openUntil) {
		t.Fatalf("failures=%d openUntil=%v, want %d %v", c.failures, c.openUntil, failures, openUntil)
	}
	if !c.probing {
		t.Fatal("a late outcome of an older call must not free the probe it does not hold")
	}
}

func TestProbeOutcomeChangesTheStateAndOlderOutcomesAreIgnoredAfterwards(t *testing.T) {
	t.Run("success closes", func(t *testing.T) {
		var c Circuit
		older, probe := openWithOlderCalls(t, &c, 1)
		c.release(true, probe, time.Now())
		for range 3 {
			if _, ok := c.reserve(time.Now()); !ok {
				t.Fatal("a successful probe did not close the breaker")
			}
		}
		// Late failures of older generations must not open it again.
		for range 3 {
			c.release(false, older[0], time.Now())
		}
		if _, ok := c.reserve(time.Now()); !ok {
			t.Fatal("late failures of an older generation opened the closed breaker")
		}
	})
	t.Run("failure reopens for 15 seconds", func(t *testing.T) {
		var c Circuit
		older, probe := openWithOlderCalls(t, &c, 1)
		now := time.Now()
		c.release(false, probe, now)
		c.mu.Lock()
		openUntil := c.openUntil
		c.mu.Unlock()
		if d := openUntil.Sub(now); d != 15*time.Second {
			t.Fatalf("pause = %v, want 15s", d)
		}
		if _, ok := c.reserve(now.Add(14 * time.Second)); ok {
			t.Fatal("reopened breaker let a request through")
		}
		// A late success of an older generation does not close it.
		c.release(true, older[0], now.Add(time.Second))
		if _, ok := c.reserve(now.Add(14 * time.Second)); ok {
			t.Fatal("a late success of an older generation closed the reopened breaker")
		}
		next, ok := c.reserve(now.Add(15 * time.Second))
		if !ok || !next.probe {
			t.Fatalf("reserve after the pause = (%+v, %v), want the probe", next, ok)
		}
	})
}

func TestProbeKeepsItsGenerationWhenAnotherReservationIsReleased(t *testing.T) {
	var c Circuit
	_, probe := openWithOlderCalls(t, &c, 0)
	c.releaseReservation()
	again, ok := c.reserve(time.Now())
	if !ok || !again.probe || again.generation != probe.generation {
		t.Fatalf("reserve = (%+v, %v), want a probe of generation %d", again, ok, probe.generation)
	}
	c.release(true, again, time.Now())
	if _, ok := c.reserve(time.Now()); !ok {
		t.Fatal("the probe outcome was ignored")
	}
}

func TestClosedStateCountsFailuresOfOneGeneration(t *testing.T) {
	var c Circuit
	now := time.Now()
	fail := func() {
		ticket, ok := c.reserve(now)
		if !ok {
			t.Fatal("closed breaker rejected a call")
		}
		c.release(false, ticket, now)
	}
	fail()
	fail()
	ticket, _ := c.reserve(now)
	c.release(true, ticket, now)
	fail()
	fail()
	if _, ok := c.reserve(now); !ok {
		t.Fatal("two failures after a success opened the breaker")
	}
	fail()
	if _, ok := c.reserve(now); ok {
		t.Fatal("three failures did not open the breaker")
	}
}

func TestLateSuccessThroughCallDoesNotCloseTheHalfOpenBreaker(t *testing.T) {
	proceed := make(chan struct{})
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-proceed
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "m", Timeout: 5 * time.Second}
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	cb := caller.circuit("model-a")
	done := make(chan *UpstreamFailure, 1)
	go func() { _, f := caller.call(context.Background(), in, request, "r", false); done <- f }()
	<-started
	past := time.Now().Add(-time.Minute)
	for range 3 {
		ticket, ok := cb.reserve(past)
		if !ok {
			t.Fatal("closed breaker rejected a call")
		}
		cb.release(false, ticket, past)
	}
	if ticket, ok := cb.reserve(time.Now()); !ok || !ticket.probe {
		t.Fatalf("reserve = (%+v, %v), want the probe", ticket, ok)
	}
	close(proceed)
	if failure := <-done; failure != nil {
		t.Fatalf("call failure = %+v", failure)
	}
	if _, ok := cb.reserve(time.Now()); ok {
		t.Fatal("a late 200 of a call started before opening closed the half-open breaker")
	}
}

func TestStressOlderCallsFinishDuringTheProbe(t *testing.T) {
	for round := range 50 {
		var c Circuit
		const n = 40
		older, probe := openWithOlderCalls(t, &c, n)
		var granted, probes atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, ticket := range older {
			wg.Add(2)
			ok := rand.New(rand.NewSource(int64(round*n+i))).Intn(2) == 0
			go func() {
				defer wg.Done()
				<-start
				c.release(ok, ticket, time.Now())
			}()
			go func() {
				defer wg.Done()
				<-start
				if tk, got := c.reserve(time.Now()); got {
					granted.Add(1)
					if tk.probe {
						probes.Add(1)
					}
				}
			}()
		}
		close(start)
		wg.Wait()
		if granted.Load() != 0 || probes.Load() != 0 {
			t.Fatalf("round %d: %d reservations (%d probes) before the outcome of the probe", round, granted.Load(), probes.Load())
		}
		c.release(true, probe, time.Now())
		if _, ok := c.reserve(time.Now()); !ok {
			t.Fatalf("round %d: breaker not closed after the probe success", round)
		}
	}
}
