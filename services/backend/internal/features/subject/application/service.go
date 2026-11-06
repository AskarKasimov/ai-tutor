package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/logx"
	"go.uber.org/zap"
)

type Repository interface {
	Create(context.Context, string, string, int64) (subject.Subject, error)
	List(context.Context) ([]subject.Subject, error)
	AudioPreparationState(context.Context, string) (subjectExists, graphPublished bool, err error)
}

type IDGenerator interface {
	New(prefix string) (string, error)
}

type Service struct {
	repo              Repository
	ids               IDGenerator
	now               func() time.Time
	examAudio         ExamProgressReader
	learnAudio        LearningProgressReader
	learningMu        sync.Mutex
	learningSnapshots map[string]audiopreparation.Learning
	learningSequence  uint64
	learningCompleted map[string]uint64
	examNoGraphMu     sync.Mutex
	examNoGraphAt     map[string]time.Time
	refreshExamAudio  func(context.Context, string) error
}

type ExamProgressReader interface {
	Snapshot(string) (audiopreparation.ExamSnapshot, bool)
	VoiceName(context.Context) (string, error)
	RecoveryComplete() bool
}

type LearningProgressReader interface {
	Snapshot(context.Context, string) (int64, []audiopreparation.LearningAsset, error)
}

func (s *Service) SetAudioProgress(exam ExamProgressReader, learning LearningProgressReader) {
	s.examAudio, s.learnAudio = exam, learning
}

func (s *Service) SetExamAudioRefresh(refresh func(context.Context, string) error) {
	s.refreshExamAudio = refresh
}

func (s *Service) AudioPreparation(ctx context.Context, actor user.User, subjectID, direction string) (audiopreparation.Response, error) {
	if actor.Role != user.Admin {
		return audiopreparation.Response{}, fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	if s.examAudio == nil || s.learnAudio == nil {
		return audiopreparation.Response{}, fmt.Errorf("audio preparation readers are not configured")
	}
	subjectExists, graphPublished, err := s.repo.AudioPreparationState(ctx, subjectID)
	if err != nil {
		return audiopreparation.Response{}, err
	}
	if !subjectExists {
		return audiopreparation.Response{}, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Курс не найден.")
	}
	result := audiopreparation.Response{SubjectID: subjectID}
	if direction != "learn" {
		result.Exam, err = s.examAudioPreparation(ctx, subjectID, graphPublished)
		if err != nil {
			logx.From(ctx).Warn("exam audio progress unavailable", zap.String("subject_id", subjectID), zap.Error(err))
		}
	}
	if direction != "exam" {
		result.Learn, err = s.learningAudioPreparation(ctx, subjectID)
		if err != nil {
			logx.From(ctx).Warn("learning audio progress unavailable", zap.String("subject_id", subjectID), zap.Error(err))
		}
	}

	if ctx.Err() != nil {
		return audiopreparation.Response{}, ctx.Err()
	}
	return result, nil
}

func (s *Service) examAudioPreparation(ctx context.Context, subjectID string, graphPublished bool) (*audiopreparation.Exam, error) {
	var exam *audiopreparation.Exam
	snapshot, ok := s.examAudio.Snapshot(subjectID)
	recovered := s.examAudio.RecoveryComplete()
	if !graphPublished {
		ok = false
	}
	if graphPublished && !ok && recovered && s.refreshExamAudio != nil {
		if err := s.refreshExamAudio(ctx, subjectID); err != nil {
			return nil, err
		}
		snapshot, ok = s.examAudio.Snapshot(subjectID)
	}
	if !ok && recovered && graphPublished {
		// Recovery may have registered this subject after the first read.
		snapshot, ok = s.examAudio.Snapshot(subjectID)
	}
	if ok {
		voice, err := s.examAudio.VoiceName(ctx)
		if err != nil {
			return nil, err
		}
		snapshot.Version += voice
		exam = &audiopreparation.Exam{Direction: audiopreparation.Direction{Version: snapshot.Version, Total: snapshot.Total, Counts: snapshot.Counts, ObservedAt: snapshot.ObservedAt}, Published: true}
		exam.State = snapshot.Counts.State(true, snapshot.Total)
	} else {
		if !recovered || graphPublished {
			return nil, fault.New(fault.Upstream, "AUDIO_RECOVERY_IN_PROGRESS", "Прогресс озвучки ещё восстанавливается.")
		}
		s.examNoGraphMu.Lock()
		if s.examNoGraphAt == nil {
			s.examNoGraphAt = make(map[string]time.Time)
		}
		observedAt, exists := s.examNoGraphAt[subjectID]
		if !exists {
			observedAt = s.now()
			s.examNoGraphAt[subjectID] = observedAt
		}
		s.examNoGraphMu.Unlock()
		exam = &audiopreparation.Exam{Published: false, Direction: audiopreparation.Direction{State: audiopreparation.NoGraph, ObservedAt: observedAt}}
	}
	if ok {
		s.examNoGraphMu.Lock()
		delete(s.examNoGraphAt, subjectID)
		s.examNoGraphMu.Unlock()
	}
	return exam, nil
}

func (s *Service) learningAudioPreparation(ctx context.Context, subjectID string) (*audiopreparation.Learning, error) {
	s.learningMu.Lock()
	s.learningSequence++
	sequence := s.learningSequence
	s.learningMu.Unlock()
	revision, assets, err := s.learnAudio.Snapshot(ctx, subjectID)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("learning audio progress invariant violated: %d counts for %d assets", counts.Sum(), len(seen))
	}
	learning := &audiopreparation.Learning{Revision: revision, Direction: audiopreparation.Direction{Version: fmt.Sprintf("revision:%d", revision), State: counts.State(true, len(seen)), Total: len(seen), Counts: counts, ObservedAt: observedAt}}
	s.learningMu.Lock()
	defer s.learningMu.Unlock()
	if previous, ok := s.learningSnapshots[subjectID]; ok && previous.Revision == revision && previous.Total == learning.Total && previous.Counts == counts {
		learning.ObservedAt = previous.ObservedAt
	}
	if s.learningSnapshots == nil {
		s.learningSnapshots = make(map[string]audiopreparation.Learning)
	}
	if s.learningCompleted == nil {
		s.learningCompleted = make(map[string]uint64)
	}
	if sequence > s.learningCompleted[subjectID] {
		s.learningSnapshots[subjectID] = *learning
		s.learningCompleted[subjectID] = sequence
	}
	return learning, nil
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
