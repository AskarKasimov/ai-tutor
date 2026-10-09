package application

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Repository interface {
	Create(context.Context, string, string, int64) (subject.Subject, error)
	List(context.Context) ([]subject.Subject, error)
}

type IDGenerator interface {
	New(prefix string) (string, error)
}

type Service struct {
	repo Repository
	ids  IDGenerator
	now  func() time.Time
}

func New(repo Repository, ids IDGenerator, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, ids: ids, now: now}
}

func (s *Service) Create(ctx context.Context, actor user.User, name string) (subject.Subject, error) {
	if actor.Role != user.Admin {
		return subject.Subject{}, fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return subject.Subject{}, fault.Validation("name", "Название предмета должно содержать от 1 до 200 символов.")
	}
	id, err := s.ids.New("subject")
	if err != nil {
		return subject.Subject{}, err
	}
	return s.repo.Create(ctx, id, name, s.now().Unix())
}

func (s *Service) Catalog(ctx context.Context, actor user.User) ([]subject.Subject, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Role == user.Admin {
		return items, nil
	}
	ready := make([]subject.Subject, 0, len(items))
	for _, item := range items {
		if item.Ready {
			ready = append(ready, item)
		}
	}
	return ready, nil
}
