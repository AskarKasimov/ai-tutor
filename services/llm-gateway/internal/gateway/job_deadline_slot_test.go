package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// jobDeadlineCases are wire errors reported after the job deadline ended the
// call context.
var jobDeadlineCases = []struct {
	name string
	tr   deadlineErrTransport
	code string
}{
	{"body cut short", deadlineErrTransport{body: true, err: io.ErrUnexpectedEOF}, "upstream_response_error"},
	{"connection reset after send", deadlineErrTransport{err: errors.New("read tcp: connection reset by peer")}, "upstream_transport_error"},
}

func TestJobDeadlineBrokenConnectionReleasesVerifiedSlot(t *testing.T) {
	for _, tc := range jobDeadlineCases {
		for _, abort := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s abort=%v", tc.name, abort), func(t *testing.T) {
				in := timeoutInstance(abort)
				in.Timeout = 5 * time.Second
				caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
				caller.client.Transport = tc.tr
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
				_, failure := caller.call(ctx, in, timeoutRequest(t), "r", false)
				cancel()
				if failure == nil || !failure.JobDeadline || failure.InstanceTimeout || failure.Code != tc.code || failure.BeforeSend {
					t.Fatalf("failure = %+v, want JobDeadline with %s sent", failure, tc.code)
				}
				if got := uncertainUpstreamOutcomeForInstance(in, failure); got == abort {
					t.Fatalf("abort=%v: uncertain = %v", abort, got)
				}

				s, _ := repairFixture(t)
				s.caller.client.Transport = tc.tr
				s.cfg.Instances[0] = in
				s.cfg.Instances[0].MaxInFlight = 2
				j := runningJob(s, "j", time.Now().Add(30*time.Millisecond))
				s.execute(context.Background(), j, s.cfg.Instances[0])

				if e := s.job("j").Error; e == nil || e.Status != 504 || e.Code != "request_deadline_exceeded" {
					t.Fatalf("abort=%v stored error = %+v, want 504 request_deadline_exceeded", abort, e)
				}
				want := 2 // the bystander and the job: the outcome stays uncertain
				if abort {
					want = 1 // our deadline closed the request: the model got the cancel
				}
				if got := s.slots(); got != want {
					t.Fatalf("abort=%v in flight = %d, want %d", abort, got, want)
				}
			})
		}
	}
}

func TestJobDeadlineBeforeSendIsNotUncertain(t *testing.T) {
	failure := &UpstreamFailure{Code: "upstream_transport_error", Err: errors.New("dial tcp: connection refused"), BeforeSend: true, JobDeadline: true}
	if uncertainUpstreamOutcomeForInstance(timeoutInstance(true), failure) || uncertainUpstreamOutcomeForInstance(timeoutInstance(false), failure) {
		t.Fatal("a failure before send at the job deadline must not be an uncertain outcome")
	}
}
