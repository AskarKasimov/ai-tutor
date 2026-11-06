package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// blockingResponseWriter блокируется на первом Write до сигнала release.
type blockingResponseWriter struct {
	header  http.Header
	entered chan struct{}
	release chan struct{}
	once    bool
}

func (w *blockingResponseWriter) Header() http.Header { return w.header }
func (w *blockingResponseWriter) WriteHeader(int)     {}
func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	if !w.once {
		w.once = true
		close(w.entered)
	}
	<-w.release
	return len(p), nil
}

func TestMetricsDoNotHoldSchedulerLockWhileWriting(t *testing.T) {
	s := schedulerFixture([]InstanceConfig{schedulerInstance("model-a")}, []string{"model-a"})
	s.started = time.Now()
	s.connections = make(chan struct{}, 1)
	w := &blockingResponseWriter{header: http.Header{}, entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.writeMetrics(w)
	}()
	defer func() {
		close(w.release)
		<-done
	}()
	select {
	case <-w.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("writeMetrics did not start writing")
	}
	locked := make(chan struct{})
	go func() {
		s.Ready()
		close(locked)
	}()
	select {
	case <-locked:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("s.mu is held while the metrics response is being written")
	}
}

func TestMetricsContent(t *testing.T) {
	instances := []InstanceConfig{schedulerInstance("model-a")}
	s := schedulerFixture(instances, []string{"model-a"})
	s.started = time.Now()
	s.connections = make(chan struct{}, 2)
	s.connections <- struct{}{}
	s.expected = map[string]float64{"model-a": 1.5}
	s.inFlight["model-a"] = 1
	s.routeSelections = map[routeSelection]uint64{{complexity: "low", instance: "model-a"}: 3}
	s.idempotentReplays = 2
	now := time.Now()
	s.jobs["q"] = schedulerJob("q", 1, true)
	s.jobs["q"].SubmittedAt = now.Add(-time.Hour)
	done := schedulerJob("done", 2, false)
	done.Status = "succeeded"
	done.Attempt = 3
	done.SubmittedAt = now.Add(-10 * time.Second)
	done.StartedAt = now.Add(-8 * time.Second)
	done.FinishedAt = now.Add(-4 * time.Second)
	s.jobs["done"] = done
	s.recordCall("http", 200)
	s.recordCall("grpc", 429)

	rec := httptest.NewRecorder()
	s.writeMetrics(rec)
	body := rec.Body.String()
	for _, want := range []string{
		"gateway_queue_depth{complexity=\"low\",urgent=\"true\"} 1\n",
		"gateway_queue_depth{complexity=\"low\",urgent=\"false\"} 0\n",
		"gateway_instance_in_flight{instance=\"model-a\"} 1\ngateway_instance_capacity{instance=\"model-a\"} 1\ngateway_circuit_open{instance=\"model-a\"} 0\n",
		"gateway_instance_expected_seconds{instance=\"model-a\"} 1.500\n",
		"gateway_route_eligible{complexity=\"low\",instance=\"model-a\",model=\"model-a\",tier=\"high\"} 1\n",
		"gateway_route_selections_total{complexity=\"low\",instance=\"model-a\",model=\"model-a\",tier=\"high\"} 3\n",
		"gateway_route_selections_total{complexity=\"high\",instance=\"model-a\",model=\"model-a\",tier=\"high\"} 0\n",
		"gateway_calls_total{transport=\"http\",status=\"200\"} 1\n",
		"gateway_calls_total{transport=\"grpc\",status=\"429\"} 1\n",
		"gateway_requests_total{status=\"succeeded\"} 1\n",
		"gateway_requests_total{status=\"failed\"} 0\n",
		"gateway_idempotent_replays_total 2\ngateway_retries_total 2\ngateway_connections_in_flight 1\ngateway_unregistered_requests 0\n",
		"gateway_queue_wait_seconds_bucket{le=\"1\"} 0\n",
		"gateway_queue_wait_seconds_bucket{le=\"5\"} 1\n",
		"gateway_queue_wait_seconds_count 1\n",
		"gateway_execution_seconds_bucket{le=\"5\"} 1\n",
		"gateway_execution_seconds_bucket{le=\"+Inf\"} 1\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
	if !strings.Contains(body, "gateway_queue_oldest_seconds{complexity=\"low\",urgent=\"true\"} 3600.") {
		t.Errorf("oldest queued age not ~3600s:\n%s", body)
	}
}
