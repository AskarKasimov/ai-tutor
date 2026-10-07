package application

import (
	"context"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Service struct {
	reader      ResultReader
	synthesizer FeedbackSynthesizer
	cache       FeedbackCache
	taskFinder  TaskMetadataFinder
	now         func() time.Time
}

func New(reader ResultReader, synthesizer FeedbackSynthesizer, cache FeedbackCache, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		reader:      reader,
		synthesizer: synthesizer,
		cache:       cache,
		now:         now,
	}
}

func (s *Service) WithTaskFinder(finder TaskMetadataFinder) *Service {
	s.taskFinder = finder
	return s
}

func (s *Service) GetFeedback(ctx context.Context, ownerID, sessionID string) (diagnostic.OverallFeedback, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(sessionID) > 128 {
		return diagnostic.OverallFeedback{}, fault.Validation("id", "Укажите корректный ID диагностической сессии.")
	}

	if s.cache != nil {
		if cached, ok := s.cache.Get(ctx, sessionID); ok && cached != nil {
			return *cached, nil
		}
	}

	result, err := s.reader.Result(ctx, ownerID, sessionID)
	if err != nil {
		return diagnostic.OverallFeedback{}, err
	}

	report := AnalyzeResult(result)

	summary, err := s.synthesizer.Synthesize(ctx, report)
	if err != nil {
		return diagnostic.OverallFeedback{}, err
	}

	feedback := diagnostic.OverallFeedback{
		SessionID:               result.SessionID,
		DiagnosticScore:         report.DiagnosticScore,
		MaximumScore:            report.MaximumScore,
		ScorePercentage:         report.ScorePercentage,
		Summary:                 summary,
		Strengths:               report.Strengths,
		ConfirmedGaps:           report.ConfirmedGaps,
		PartialCompetencies:     report.PartialCompetencies,
		UnverifiedCompetencies:  report.UnverifiedCompetencies,
		TrainingRecommendations: report.TrainingRecommendations,
		GeneratedAt:             s.now().Unix(),
	}

	if s.cache != nil {
		s.cache.Set(ctx, sessionID, feedback)
	}

	return feedback, nil
}

type OverallAnswersFeedbackResponse struct {
	Score           int      `json:"score"`
	MaxScore        int      `json:"max_score"`
	ScorePercentage int      `json:"score_percentage"`
	Summary         string   `json:"summary"`
	Strengths       []string `json:"strengths"`
	Gaps            []string `json:"gaps"`
	Partials        []string `json:"partials"`
	Recommendations []string `json:"recommendations"`
	GeneratedAt     int64    `json:"generated_at"`
}

func (s *Service) GenerateAnswersFeedback(ctx context.Context, items []AnswerFeedbackItem) (OverallAnswersFeedbackResponse, error) {
	if len(items) == 0 {
		return OverallAnswersFeedbackResponse{}, fault.Validation("answers", "Список ответов не может быть пустым.")
	}

	if s.taskFinder != nil {
		for i := range items {
			if items[i].TaskID != "" {
				compName, outcomeName, content, err := s.taskFinder.FindTaskMetadata(ctx, items[i].TaskID)
				if err == nil {
					if compName != "" && (items[i].Topic == "" || strings.HasPrefix(items[i].Topic, "ml_")) {
						items[i].Topic = compName
					}
					if items[i].EducationalContent == "" && content != "" {
						items[i].EducationalContent = content
					}
					if items[i].OutcomeName == "" && outcomeName != "" {
						items[i].OutcomeName = outcomeName
					}
				}
			}
		}
	}

	report := AnalyzeAnswers(items)

	summary, err := s.synthesizer.SynthesizeAnswers(ctx, report)
	if err != nil {
		return OverallAnswersFeedbackResponse{}, err
	}

	return OverallAnswersFeedbackResponse{
		Score:           report.Score,
		MaxScore:        report.MaxScore,
		ScorePercentage: report.ScorePercentage,
		Summary:         summary,
		Strengths:       report.Strengths,
		Gaps:            report.Gaps,
		Partials:        report.Partials,
		Recommendations: report.Recommendations,
		GeneratedAt:     s.now().Unix(),
	}, nil
}
