package http

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"

type ConfirmedGapResponse struct {
	CompetencyID   string   `json:"competency_id" binding:"required"`
	CompetencyName string   `json:"competency_name" binding:"required"`
	OutcomeID      string   `json:"outcome_id" binding:"required"`
	OutcomeName    string   `json:"outcome_name" binding:"required"`
	TaxonomyCode   string   `json:"taxonomy_code" binding:"required"`
	Importance     int16    `json:"importance" binding:"required"`
	FailedCriteria []string `json:"failed_criteria" binding:"required"`
	Advice         string   `json:"advice" binding:"required"`
}

type PartialCompetencyResponse struct {
	CompetencyID   string `json:"competency_id" binding:"required"`
	CompetencyName string `json:"competency_name" binding:"required"`
	Details        string `json:"details" binding:"required"`
}

type UnverifiedCompetencyResponse struct {
	ID   string `json:"competency_id" binding:"required"`
	Name string `json:"competency_name" binding:"required"`
	Code string `json:"code" binding:"required"`
}

type TrainingRecommendationResponse struct {
	CompetencyID   string `json:"competency_id" binding:"required"`
	CompetencyName string `json:"competency_name" binding:"required"`
	OutcomeID      string `json:"outcome_id" binding:"required"`
	OutcomeName    string `json:"outcome_name" binding:"required"`
	Priority       int    `json:"priority" minimum:"1" binding:"required"`
	Rationale      string `json:"rationale" binding:"required"`
}

type OverallFeedbackResponse struct {
	SessionID               string                           `json:"session_id" binding:"required"`
	DiagnosticScore         int                              `json:"diagnostic_score" minimum:"0" binding:"required"`
	MaximumScore            int                              `json:"maximum_score" minimum:"0" binding:"required"`
	ScorePercentage         int                              `json:"score_percentage" minimum:"0" maximum:"100" binding:"required"`
	Summary                 string                           `json:"summary" binding:"required"`
	Strengths               []string                         `json:"strengths" binding:"required"`
	ConfirmedGaps           []ConfirmedGapResponse           `json:"confirmed_gaps" binding:"required"`
	PartialCompetencies     []PartialCompetencyResponse      `json:"partial_competencies" binding:"required"`
	UnverifiedCompetencies  []UnverifiedCompetencyResponse   `json:"unverified_competencies" binding:"required"`
	TrainingRecommendations []TrainingRecommendationResponse `json:"training_recommendations" binding:"required"`
	GeneratedAt             int64                            `json:"generated_at" binding:"required"`
}

func feedbackResponse(fb diagnostic.OverallFeedback) OverallFeedbackResponse {
	gaps := make([]ConfirmedGapResponse, 0, len(fb.ConfirmedGaps))
	for _, g := range fb.ConfirmedGaps {
		gaps = append(gaps, ConfirmedGapResponse{
			CompetencyID:   g.CompetencyID,
			CompetencyName: g.CompetencyName,
			OutcomeID:      g.OutcomeID,
			OutcomeName:    g.OutcomeName,
			TaxonomyCode:   g.TaxonomyCode,
			Importance:     g.Importance,
			FailedCriteria: append([]string(nil), g.FailedCriteria...),
			Advice:         g.Advice,
		})
	}

	partials := make([]PartialCompetencyResponse, 0, len(fb.PartialCompetencies))
	for _, p := range fb.PartialCompetencies {
		partials = append(partials, PartialCompetencyResponse{
			CompetencyID:   p.CompetencyID,
			CompetencyName: p.CompetencyName,
			Details:        p.Details,
		})
	}

	unverified := make([]UnverifiedCompetencyResponse, 0, len(fb.UnverifiedCompetencies))
	for _, u := range fb.UnverifiedCompetencies {
		unverified = append(unverified, UnverifiedCompetencyResponse{
			ID:   u.ID,
			Name: u.Name,
			Code: u.Code,
		})
	}

	recs := make([]TrainingRecommendationResponse, 0, len(fb.TrainingRecommendations))
	for _, r := range fb.TrainingRecommendations {
		recs = append(recs, TrainingRecommendationResponse{
			CompetencyID:   r.CompetencyID,
			CompetencyName: r.CompetencyName,
			OutcomeID:      r.OutcomeID,
			OutcomeName:    r.OutcomeName,
			Priority:       r.Priority,
			Rationale:      r.Rationale,
		})
	}

	return OverallFeedbackResponse{
		SessionID:               fb.SessionID,
		DiagnosticScore:         fb.DiagnosticScore,
		MaximumScore:            fb.MaximumScore,
		ScorePercentage:         fb.ScorePercentage,
		Summary:                 fb.Summary,
		Strengths:               append([]string(nil), fb.Strengths...),
		ConfirmedGaps:           gaps,
		PartialCompetencies:     partials,
		UnverifiedCompetencies:  unverified,
		TrainingRecommendations: recs,
		GeneratedAt:             fb.GeneratedAt,
	}
}

type AnswerItemDTO struct {
	TaskID     string   `json:"task_id"`
	Topic      string   `json:"topic,omitempty"`
	Question   string   `json:"question"`
	Transcript string   `json:"transcript"`
	Score      int      `json:"score"`
	MaxScore   int      `json:"max_score"`
	Feedback   []string `json:"feedback"`
}

type AnswersFeedbackRequest struct {
	Answers []AnswerItemDTO `json:"answers" binding:"required"`
}

type AnswersFeedbackResponse struct {
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
