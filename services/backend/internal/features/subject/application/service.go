package application

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
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
	repo       Repository
	ids        IDGenerator
	now        func() time.Time
	examAudio  ExamProgressReader
	learnAudio LearningProgressReader
}

type ExamProgressReader interface {
	Snapshot(string) (audiopreparation.ExamSnapshot, bool)
}

type LearningProgressReader interface {
	Snapshot(context.Context, string) (int64, []audiopreparation.LearningAsset, error)
}

func (s *Service) SetAudioProgress(exam ExamProgressReader, learning LearningProgressReader) {
	s.examAudio, s.learnAudio = exam, learning
}

func (s *Service) AudioPreparation(ctx context.Context, actor user.User, subjectID string) (audiopreparation.Response, error) {
	if actor.Role != user.Admin {
		return audiopreparation.Response{}, fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	if s.examAudio == nil || s.learnAudio == nil {
		return audiopreparation.Response{}, fmt.Errorf("audio preparation readers are not configured")
	}
	result := audiopreparation.Response{SubjectID: subjectID}
	if snapshot, ok := s.examAudio.Snapshot(subjectID); ok {
		result.Exam = audiopreparation.Exam{Direction: audiopreparation.Direction{Version: snapshot.Version, Total: snapshot.Total, Counts: snapshot.Counts, ObservedAt: snapshot.ObservedAt}, Published: true}
		result.Exam.State = snapshot.Counts.State(true, snapshot.Total)
	} else {
		result.Exam = audiopreparation.Exam{Published: false, Direction: audiopreparation.Direction{State: audiopreparation.NoGraph, ObservedAt: s.now()}}
	}
	revision, assets, err := s.learnAudio.Snapshot(ctx, subjectID)
	if err != nil {
		return audiopreparation.Response{}, err
	}
	observedAt := s.now()
	counts := audiopreparation.Counts{}
	seen := map[string]bool{}
	for _, asset := range assets {
		if !audioasset.CanSynthesize(asset.Instruction) {
			continue
		}
		key := asset.TaskID
		if asset.AssetID != nil && *asset.AssetID != "" {
			key = *asset.AssetID
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch {
		case asset.AssetID == nil || asset.Status == nil || *asset.Status == string(audioasset.Cancelled) || *asset.Status == string(audioasset.Missing):
			counts.Unavailable++
		case *asset.Status == string(audioasset.Ready):
			counts.Ready++
		case *asset.Status == string(audioasset.Failed):
			counts.Failed++
		case *asset.Status == string(audioasset.Processing):
			if asset.LeaseUntil != nil && asset.LeaseUntil.After(observedAt) {
				counts.Processing++
			} else {
				counts.Queued++
			}
		case *asset.Status == string(audioasset.Pending):
			if asset.Attempts > 0 && asset.NextAttemptAt != nil && asset.NextAttemptAt.After(observedAt) {
				counts.RetryWait++
			} else {
				counts.Queued++
			}
		case *asset.Status == string(audioasset.Cancelled), *asset.Status == string(audioasset.Missing):
			counts.Unavailable++
		default:
			counts.Unavailable++
		}
	}
	if counts.Sum() != len(seen) {
		return audiopreparation.Response{}, fmt.Errorf("learning audio progress invariant violated: %d counts for %d assets", counts.Sum(), len(seen))
	}
	result.Learn = audiopreparation.Learning{Revision: revision, Direction: audiopreparation.Direction{Version: fmt.Sprintf("revision:%d", revision), State: counts.State(true, len(seen)), Total: len(seen), Counts: counts, ObservedAt: observedAt}}
	return result, nil
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
	created, err := s.repo.Create(ctx, id, name, s.now().Unix())
	if err != nil {
		return subject.Subject{}, err
	}
	created.Preparation = subject.EmptyPreparation()
	return created, nil
}

func (s *Service) Catalog(ctx context.Context, actor user.User) ([]subject.Subject, error) {
	return s.catalog(ctx, actor, false)
}

func (s *Service) LearningCatalog(ctx context.Context, actor user.User) ([]subject.Subject, error) {
	return s.catalog(ctx, actor, true)
}

func (s *Service) catalog(ctx context.Context, actor user.User, learning bool) ([]subject.Subject, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ready := make([]subject.Subject, 0, len(items))
	for _, item := range items {
		item.Ready = item.ExamReady
		if learning {
			item.Ready = item.LearningReady
		}
		if actor.Role == user.Admin {
			ready = append(ready, item)
		} else if item.Ready {
			item.Preparation = nil
			ready = append(ready, item)
		}
	}
	return ready, nil
}
