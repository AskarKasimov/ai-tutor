package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// instanceTimeoutCases are wire errors reported after the instance timeout
// ended the call context.
var instanceTimeoutCases = []struct {
	name string
	tr   deadlineErrTransport
	code string
}{
	{"body cut short", deadlineErrTransport{body: true, err: io.ErrUnexpectedEOF}, "upstream_response_error"},
	{"connection reset after send", deadlineErrTransport{err: errors.New("read tcp: connection reset by peer")}, "upstream_transport_error"},
}

func timeoutInstance(abort bool) InstanceConfig {
	return InstanceConfig{ID: "model-a", BaseURL: "http://upstream.invalid/v1", Model: "m", Timeout: 20 * time.Millisecond, AbortOnDisconnect: abort}
}

func timeoutRequest(t *testing.T) Request {
	t.Helper()
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestInstanceTimeoutReportedAsBrokenConnectionIsAnInstanceTimeout(t *testing.T) {
	for _, tc := range instanceTimeoutCases {
		t.Run(tc.name, func(t *testing.T) {
			caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
			caller.client.Transport = tc.tr
			in := timeoutInstance(false)
			request := timeoutRequest(t)
			for i := range 3 {
				if !caller.circuit("model-a").available(time.Now()) {
					t.Fatalf("call %d: breaker opened too early", i)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				_, failure := caller.call(ctx, in, request, "r", false)
				cancel()
				if failure == nil || !failure.InstanceTimeout || failure.JobDeadline || failure.Code != tc.code || failure.BeforeSend {
					t.Fatalf("call %d failure = %+v, want InstanceTimeout with %s sent", i, failure, tc.code)
				}
				if status, _, code, _ := upstreamFailureResponse(failure); status != 504 || code != "upstream_timeout" {
					t.Fatalf("call %d response = %d %s, want 504 upstream_timeout", i, status, code)
				}
				s := tieredFixture()
				s.observeCall(schedulerJob("j", 1, false), schedulerInstance("fast"), failure, time.Second)
				if _, cooling := s.cooling["fast"]; cooling || s.expected["fast"] != 1 {
					t.Fatalf("call %d: cooling=%v expected=%v, want a duration sample and no cooldown", i, s.cooling, s.expected)
				}
			}
			if caller.circuit("model-a").available(time.Now()) {
				t.Fatal("three instance timeouts did not open the breaker")
			}
		})
	}
}

func TestInstanceTimeoutBrokenConnectionSlotAndCounter(t *testing.T) {
	for _, tc := range instanceTimeoutCases {
		for _, abort := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s abort=%v", tc.name, abort), func(t *testing.T) {
				s, _ := repairFixture(t)
				s.caller.client.Transport = tc.tr
				s.cfg.Instances[0] = timeoutInstance(abort)
				s.cfg.Instances[0].MaxInFlight = 2
				j := runningJob(s, "j", time.Now().Add(time.Minute))
				s.execute(context.Background(), j, s.cfg.Instances[0])

				if e := s.job("j").Error; e == nil || e.Status != 504 || e.Code != "upstream_timeout" {
					t.Fatalf("abort=%v stored error = %+v, want 504 upstream_timeout", abort, e)
				}
				s.mu.Lock()
				counted := s.upstreamTimeouts["instance_timeout"]
				s.mu.Unlock()
				if counted != 1 {
					t.Fatalf("abort=%v instance_timeout counter = %d, want 1", abort, counted)
				}
				want := 2 // the bystander and the job: the outcome stays uncertain
				if abort {
					want = 1 // our timer closed the connection: the model got the cancel
				}
				if got := s.slots(); got != want {
					t.Fatalf("abort=%v in flight = %d, want %d", abort, got, want)
				}
			})
		}
	}
}

func TestBrokenConnectionBeforeSendAfterInstanceTimeoutStaysRetryable(t *testing.T) {
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	caller.client.Transport = deadlineErrTransport{err: errors.New("dial tcp 10.0.0.1:80: connect: connection refused")}
	failure := func() *UpstreamFailure {
		_, f := caller.call(context.Background(), timeoutInstance(false), timeoutRequest(t), "r", false)
		return f
	}()
	if failure == nil || !failure.BeforeSend || failure.JobDeadline || failure.Code != "upstream_transport_error" {
		t.Fatalf("failure = %+v, want BeforeSend transport error", failure)
	}
	if uncertainUpstreamOutcomeForInstance(timeoutInstance(false), failure) {
		t.Fatal("a failure before send must not be an uncertain outcome")
	}
	s := tieredFixture()
	s.observeCall(schedulerJob("j", 1, false), schedulerInstance("fast"), failure, time.Second)
	if _, cooling := s.cooling["fast"]; !cooling {
		t.Fatal("a failure before send must put the instance on cooldown")
	}
}
