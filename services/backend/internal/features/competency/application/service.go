// Package application implements competency-map import policy.
package application

import (
	"context"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Repository interface {
	Replace(ctx context.Context, actorID string, parsed competencymap.Map, importedAt int64) (competencymap.ImportResult, error)
}

type Parser interface {
	Parse(data []byte) (competencymap.Map, error)
}

type Service struct {
	repo   Repository
	parser Parser
	now    func() time.Time
}

func New(repo Repository, parser Parser, now func() time.Time) *Service {
	return &Service{repo: repo, parser: parser, now: now}
}

// Authorize allows transports to check import access before reading uploads.
func (s *Service) Authorize(actor user.User) error {
	if actor.Role != user.Admin {
		return fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	return nil
}

func (s *Service) Import(ctx context.Context, actor user.User, data []byte, media string) (competencymap.ImportResult, error) {
	if err := s.Authorize(actor); err != nil {
		return competencymap.ImportResult{}, err
	}
	media = strings.TrimSpace(strings.Split(media, ";")[0])
	if media != "text/csv" && media != "application/csv" && media != "application/vnd.ms-excel" &&
		media != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		return competencymap.ImportResult{}, fault.New(fault.Unsupported, "UNSUPPORTED_MEDIA_TYPE", "Ожидается файл CSV или XLSX.")
	}
	parsed, err := s.parser.Parse(data)
	if err != nil {
		return competencymap.ImportResult{}, err
	}
	return s.repo.Replace(ctx, actor.ID, parsed, s.now().Unix())
}
