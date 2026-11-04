package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/logx"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ClientInterceptor continues the trace in the gateway and logs every model
// call in one place, whichever feature made it. Prompts and answers contain what
// students said, so they are written only at the debug level.
func ClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if traceparent := logx.Traceparent(ctx); traceparent != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", traceparent)
	}
	var header metadata.MD
	started := time.Now()
	err := invoker(ctx, method, req, reply, cc, append(opts, grpc.Header(&header))...)
	logger := logx.From(ctx)
	fields := []zap.Field{zap.Duration("duration", time.Since(started)), zap.String("grpc_code", status.Code(err).String())}
	request, _ := req.(*pb.CompleteRequest)
	if request != nil {
		fields = append(fields, zap.String("complexity", request.GetComplexity().String()), zap.Bool("urgent", request.GetUrgent()))
	}
	response, _ := reply.(*pb.CompleteResponse)
	requestID := RequestID(header, err)
	if err == nil && response != nil {
		requestID = response.GetRequestId()
		fields = append(fields, zap.Int("response_bytes", len(response.GetCompletionJson())))
	}
	fields = append(fields, zap.String("gateway_request_id", requestID))
	if err == nil || status.Code(err) == codes.Canceled {
		logger.Info("llm call", fields...)
	} else {
		logger.Warn("llm call failed", append(fields, zap.Error(err))...)
	}
	if entry := logger.Check(zap.DebugLevel, "llm call content"); entry != nil && request != nil {
		content := []zap.Field{zap.String("gateway_request_id", requestID), zap.String("prompt", promptText(request))}
		if err == nil && response != nil {
			content = append(content, zap.ByteString("response", response.GetCompletionJson()))
		}
		entry.Write(content...)
	}
	return err
}

// SafeReason says why a model answer was rejected without quoting it. Decoding
// and validation errors can repeat keys and values the model invented, for
// example `unknown field "<text>"`, so only their shape reaches logs above debug.
func SafeReason(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		kind, _, _ := strings.Cut(typeErr.Value, " ")
		return fmt.Sprintf("field %s has type %s, want %s", typeErr.Field, kind, typeErr.Type)
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("malformed JSON at offset %d", syntaxErr.Offset)
	}
	return "answer failed validation"
}

// SafeFinishReason returns a known finish_reason. The gateway does not limit this
// field, so any other value is arbitrary text and stays out of logs and errors.
func SafeFinishReason(reason string) string {
	switch reason {
	case "stop", "length", "content_filter", "tool_calls", "function_call":
		return reason
	}
	return "unknown"
}

// RequestID is the gateway request of a call: from the x-request-id header, else
// from the request_id of the error's ErrorInfo. A transport failure before the
// gateway answered has none.
func RequestID(header metadata.MD, err error) string {
	if ids := header.Get("x-request-id"); len(ids) > 0 {
		return ids[0]
	}
	if s, ok := status.FromError(err); ok {
		for _, detail := range s.Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok && info.GetMetadata()["request_id"] != "" {
				return info.GetMetadata()["request_id"]
			}
		}
	}
	return ""
}

func promptText(request *pb.CompleteRequest) string {
	var text strings.Builder
	for _, message := range request.GetMessages() {
		text.WriteString(message.GetRole().String())
		text.WriteString(": ")
		text.WriteString(message.GetContent())
		text.WriteString("\n")
	}
	return text.String()
}
