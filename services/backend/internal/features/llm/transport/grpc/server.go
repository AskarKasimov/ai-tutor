package grpc

import (
	"context"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/llm/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/llmpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	llmpb.UnimplementedLLMServer
	Client application.Completer
}

func (s *Server) Complete(ctx context.Context, req *llmpb.CompleteRequest) (*llmpb.CompleteResponse, error) {
	if req.System == "" || req.Input == "" || len(req.System)+len(req.Input) > 128*1024 {
		return nil, status.Error(codes.InvalidArgument, "invalid prompt size")
	}
	content, err := s.Client.Complete(ctx, req.System, req.Input)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "model request failed")
	}
	return &llmpb.CompleteResponse{Content: content}, nil
}
