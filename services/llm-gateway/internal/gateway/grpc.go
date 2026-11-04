package gateway

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"math"
	"strings"
	"time"
)

type GRPCServer struct {
	pb.UnimplementedGatewayServer
	service *Service
}

func NewGRPCServer(s *Service) *GRPCServer { return &GRPCServer{service: s} }
func (g *GRPCServer) Complete(ctx context.Context, in *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	attemptID := newID()
	md, _ := metadata.FromIncomingContext(ctx)
	keys := md.Get("idempotency-key")
	if len(keys) > 1 {
		g.service.recordCall("grpc", 400)
		_ = grpc.SendHeader(ctx, metadata.Pairs("x-request-id", attemptID))
		return nil, grpcError(apiErr(400, "invalid_request_error", "invalid_idempotency_key", "Only one idempotency-key is allowed."), attemptID)
	}
	key := ""
	if len(keys) == 1 {
		key = strings.TrimSpace(keys[0])
	}
	req, err := requestFromProto(in)
	if err != nil {
		g.service.recordCall("grpc", 400)
		_ = grpc.SendHeader(ctx, metadata.Pairs("x-request-id", attemptID))
		return nil, grpcError(err, attemptID)
	}
	if int64(len(canonicalRequestJSON(req))) > g.service.cfg.MaxBodyBytes {
		g.service.recordCall("grpc", 413)
		_ = grpc.SendHeader(ctx, metadata.Pairs("x-request-id", attemptID))
		return nil, grpcError(apiErr(413, "invalid_request_error", "request_too_large", "Request body is too large."), attemptID)
	}
	result, id, err := g.service.Complete(ctx, req, key, "grpc", false, func(id string) { _ = grpc.SendHeader(ctx, metadata.Pairs("x-request-id", id)) })
	if err != nil {
		return nil, grpcError(err, id)
	}
	if result.status != 200 {
		var e ErrorEnvelope
		_ = json.Unmarshal(result.body, &e)
		return nil, grpcErrorWithRetry(statusError{status: result.status, body: e}, id, result.retryAfter)
	}
	response := &pb.CompleteResponse{RequestId: id, CompletionJson: result.body}
	if proto.Size(response) > g.service.cfg.GRPCMaxSendBytes {
		g.service.correctCall("grpc", 200, 413)
		return nil, grpcError(apiErr(413, "invalid_request_error", "response_too_large", "The saved response exceeds the gRPC send limit."), id)
	}
	return response, nil
}
func requestFromProto(in *pb.CompleteRequest) (Request, error) {
	if in == nil || len(in.Messages) == 0 {
		return Request{}, apiErr(400, "invalid_request_error", "invalid_messages", "messages must be a non-empty array")
	}
	complexity := map[pb.Complexity]string{pb.Complexity_COMPLEXITY_LOW: "low", pb.Complexity_COMPLEXITY_MEDIUM: "medium", pb.Complexity_COMPLEXITY_HIGH: "high"}[in.Complexity]
	if complexity == "" {
		return Request{}, apiErr(400, "invalid_request_error", "invalid_complexity", "complexity must be low, medium, or high")
	}
	fields := map[string]any{"complexity": complexity, "urgent": in.Urgent}
	messages := make([]map[string]any, 0, len(in.Messages))
	for _, m := range in.Messages {
		if m == nil || m.Content == nil {
			return Request{}, apiErr(400, "invalid_request_error", "invalid_messages", "message content is required")
		}
		role := map[pb.Role]string{pb.Role_ROLE_SYSTEM: "system", pb.Role_ROLE_USER: "user", pb.Role_ROLE_ASSISTANT: "assistant"}[m.Role]
		if role == "" {
			return Request{}, apiErr(400, "invalid_request_error", "invalid_messages", "message role is unsupported")
		}
		messages = append(messages, map[string]any{"role": role, "content": m.GetContent()})
	}
	fields["messages"] = messages
	if p := in.Parameters; p != nil {
		for _, f := range []struct {
			name  string
			value *float64
		}{{"temperature", p.Temperature}, {"top_p", p.TopP}, {"presence_penalty", p.PresencePenalty}, {"frequency_penalty", p.FrequencyPenalty}} {
			if f.value != nil {
				if math.IsNaN(*f.value) || math.IsInf(*f.value, 0) {
					return Request{}, apiErr(400, "invalid_request_error", "invalid_parameter", f.name+" must be finite")
				}
				fields[f.name] = *f.value
			}
		}
		for _, f := range []struct {
			name  string
			value *int64
		}{{"seed", p.Seed}, {"max_tokens", p.MaxTokens}, {"max_completion_tokens", p.MaxCompletionTokens}} {
			if f.value != nil {
				fields[f.name] = *f.value
			}
		}
		if p.Stop != nil {
			fields["stop"] = p.Stop.Values
		}
		if p.ResponseFormat != nil {
			v := map[pb.ResponseFormat]string{pb.ResponseFormat_RESPONSE_FORMAT_TEXT: "text", pb.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT: "json_object"}[*p.ResponseFormat]
			if v == "" {
				return Request{}, apiErr(400, "invalid_request_error", "unsupported_parameter", "response_format is unsupported")
			}
			fields["response_format"] = map[string]string{"type": v}
		}
		if p.ReasoningEffort != nil {
			v := map[pb.ReasoningEffort]string{pb.ReasoningEffort_REASONING_EFFORT_LOW: "low", pb.ReasoningEffort_REASONING_EFFORT_MEDIUM: "medium", pb.ReasoningEffort_REASONING_EFFORT_HIGH: "high"}[*p.ReasoningEffort]
			if v == "" {
				return Request{}, apiErr(400, "invalid_request_error", "invalid_parameter", "reasoning_effort is unsupported")
			}
			fields["reasoning_effort"] = v
		}
	}
	body, _ := json.Marshal(fields)
	return ParseRequest(body)
}
func grpcError(err error, id string) error { return grpcErrorWithRetry(err, id, 0) }
func grpcErrorWithRetry(err error, id string, retrySeconds int) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "Request canceled.")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Request deadline exceeded.")
	}
	var se statusError
	if !errors.As(err, &se) {
		se = apiErr(503, "server_error", "state_unavailable", "Gateway state is unavailable.").(statusError)
	}
	code := codes.Internal
	switch se.status {
	case 400:
		code = codes.InvalidArgument
	case 409:
		if se.body.Error.Code == "idempotency_conflict" {
			code = codes.AlreadyExists
		} else {
			code = codes.Canceled
		}
	case 413, 429:
		code = codes.ResourceExhausted
	case 503:
		code = codes.Unavailable
	case 504:
		code = codes.DeadlineExceeded
	}
	st := status.New(code, se.body.Error.Message)
	info := &errdetails.ErrorInfo{Domain: "llmgateway", Reason: se.body.Error.Code, Metadata: map[string]string{"type": se.body.Error.Type, "request_id": id}}
	if retrySeconds > 0 {
		with, e := st.WithDetails(info, &errdetails.RetryInfo{RetryDelay: durationpb.New(time.Duration(retrySeconds) * time.Second)})
		if e == nil {
			return with.Err()
		}
	}
	with, e := st.WithDetails(info)
	if e == nil {
		return with.Err()
	}
	return st.Err()
}
