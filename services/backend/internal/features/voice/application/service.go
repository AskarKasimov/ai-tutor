package application

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type Repository interface {
	Save(context.Context, transcription.Transcription) error
}

type Recognizer interface {
	Recognize(context.Context, []byte, string) (string, error)
}

type Synthesizer interface {
	Synthesize(context.Context, string) ([]byte, error)
}

type Service struct {
	repo        Repository
	recognizer  Recognizer
	synthesizer Synthesizer
	now         func() time.Time
}

func New(repo Repository, recognizer Recognizer, synthesizer Synthesizer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, recognizer: recognizer, synthesizer: synthesizer, now: now}
}

func (s *Service) Transcribe(ctx context.Context, ownerID string, data []byte, media string) (transcription.Transcription, error) {
	var tr transcription.Transcription
	if err := audio.Check(data, media); err != nil {
		return tr, err
	}
	text, err := s.recognizer.Recognize(ctx, data, media)
	if err != nil {
		return tr, err
	}
	if !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
		return tr, fault.New(fault.Upstream, "TRANSCRIPTION_FAILED", "Некорректный ответ сервиса распознавания.")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return tr, fault.New(fault.Invalid, "NO_SPEECH_DETECTED", "Речь не распознана; повторите запись.")
	}
	id, err := security.ID("transcription")
	if err != nil {
		return tr, err
	}
	tr = transcription.Transcription{ID: id, OwnerID: ownerID, Text: text, CreatedAt: s.now().Unix()}
	if err := s.repo.Save(ctx, tr); err != nil {
		return transcription.Transcription{}, err
	}
	return tr, nil
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if count := utf8.RuneCountInString(text); count < 1 || count > 500 || strings.TrimSpace(text) == "" || strings.IndexByte(text, 0) >= 0 {
		return nil, fault.Validation("text", "Текст должен содержать 1–500 символов и не быть пустым.")
	}
	return s.synthesizer.Synthesize(ctx, text)
}
