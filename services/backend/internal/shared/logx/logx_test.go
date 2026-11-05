package logx

import (
	"context"
	"regexp"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestTraceparentContinuesTheTraceOfTheContext(t *testing.T) {
	if got := Traceparent(context.Background()); got != "" {
		t.Fatalf("a context without a trace produced %q", got)
	}
	ctx := WithTrace(context.Background(), "0123456789abcdef0123456789abcdef")
	if got := Traceparent(ctx); !regexp.MustCompile(`^00-0123456789abcdef0123456789abcdef-[0-9a-f]{16}-01$`).MatchString(got) {
		t.Fatalf("traceparent = %q", got)
	}
}

func TestStartTraceLabelsLogsWithTheTraceOfTheWork(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	ctx := StartTrace(With(context.Background(), zap.New(core)))
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(TraceID(ctx)) {
		t.Fatalf("trace = %q", TraceID(ctx))
	}
	From(ctx).Info("job")
	if got := logs.All()[0].ContextMap()["trace_id"]; got != TraceID(ctx) {
		t.Fatalf("logged trace = %v, want %s", got, TraceID(ctx))
	}
}

func TestFromFallsBackToTheGlobalLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	defer zap.ReplaceGlobals(zap.New(core))()
	From(context.Background()).Info("outside a request")
	if logs.Len() != 1 {
		t.Fatalf("the failure outside a request was lost: %d entries", logs.Len())
	}
}
