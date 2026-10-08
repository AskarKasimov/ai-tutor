package diagnostic

type OverallFeedback struct {
	SessionID               string                   `json:"session_id"`
	DiagnosticScore         int                      `json:"diagnostic_score"`
	MaximumScore            int                      `json:"maximum_score"`
	ScorePercentage         int                      `json:"score_percentage"`
	Summary                 string                   `json:"summary"`
	Strengths               []string                 `json:"strengths"`
	ConfirmedGaps           []ConfirmedGap           `json:"confirmed_gaps"`
	PartialCompetencies     []PartialCompetency      `json:"partial_competencies"`
	UnverifiedCompetencies  []SkippedCompetency      `json:"unverified_competencies"`
	TrainingRecommendations []TrainingRecommendation `json:"training_recommendations"`
	GeneratedAt             int64                    `json:"generated_at"`
}

type ConfirmedGap struct {
	CompetencyID   string   `json:"competency_id"`
	CompetencyName string   `json:"competency_name"`
	OutcomeID      string   `json:"outcome_id"`
	OutcomeName    string   `json:"outcome_name"`
	TaxonomyCode   string   `json:"taxonomy_code"`
	Importance     int16    `json:"importance"`
	FailedCriteria []string `json:"failed_criteria"`
	Advice         string   `json:"advice"`
}

type PartialCompetency struct {
	CompetencyID   string `json:"competency_id"`
	CompetencyName string `json:"competency_name"`
	Details        string `json:"details"`
}

type TrainingRecommendation struct {
	CompetencyID   string `json:"competency_id"`
	CompetencyName string `json:"competency_name"`
	OutcomeID      string `json:"outcome_id"`
	OutcomeName    string `json:"outcome_name"`
	Priority       int    `json:"priority"`
	Rationale      string `json:"rationale"`
}
