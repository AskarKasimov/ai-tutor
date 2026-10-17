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
	// defaultGraph is set when Geek serves the graph fixture to every course
	// without its own graph, so a course is ready as soon as it is created.
	defaultGraph bool
}

func New(repo Repository, ids IDGenerator, now func() time.Time, defaultGraph bool) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, ids: ids, now: now, defaultGraph: defaultGraph}
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
	created, err := s.repo.Create(ctx, id, name, s.now().Unix())
	if err != nil {
		return subject.Subject{}, err
	}
	created.Ready = created.Ready || s.defaultGraph
	return created, nil
}

func (s *Service) Catalog(ctx context.Context, actor user.User) ([]subject.Subject, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Ready = items[i].Ready || s.defaultGraph
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
