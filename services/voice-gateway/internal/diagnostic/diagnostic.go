// Package diagnostic carries bounded, content-free failure categories and request identity.
package diagnostic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Error struct {
	Code       codes.Code
	Reason     string
	Stage      string
	HTTPStatus int
	Uncertain  bool
}

func (e *Error) Error() string              { return "voice request failed: " + e.Reason }
func (e *Error) GRPCStatus() *status.Status { return status.New(e.Code, e.Error()) }
func New(code codes.Code, stage, reason string) *Error {
	return &Error{Code: code, Stage: stage, Reason: reason}
}
func Info(err error) Error {
	if err == nil {
		return Error{Code: codes.OK, Reason: "ok", Stage: "complete"}
	}
	var e *Error
	if errors.As(err, &e) {
		return *e
	}
	code := status.Code(err)
	if errors.Is(err, context.Canceled) {
		code = codes.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = codes.DeadlineExceeded
	}
	reason := strings.ToLower(code.String())
	if code == codes.Unknown {
		code = codes.Unavailable
		reason = "internal_error"
	}
	return Error{Code: code, Reason: reason, Stage: "request"}
}
func Fields(err error) []slog.Attr {
	d := Info(err)
	fields := []slog.Attr{slog.String("error_code", d.Code.String()), slog.String("error", d.Reason), slog.String("stage", d.Stage)}
	if d.HTTPStatus != 0 {
		fields = append(fields, slog.Int("upstream_http_status", d.HTTPStatus))
	}
	return fields
}

type requestKey struct{}
type Request struct {
	mu sync.Mutex
	id string
}

func Begin(ctx context.Context) (context.Context, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, requestKey{}, &Request{id: hex.EncodeToString(b)}), nil
}
func ID(ctx context.Context) string {
	r, _ := ctx.Value(requestKey{}).(*Request)
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}
func SetID(ctx context.Context, id string) {
	r, _ := ctx.Value(requestKey{}).(*Request)
	if r == nil {
		return
	}
	r.mu.Lock()
	r.id = id
	r.mu.Unlock()
}

var trace = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-0[01]$`)

func TraceID(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("traceparent")
	if len(values) != 1 {
		return ""
	}
	matches := trace.FindStringSubmatch(values[0])
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}
