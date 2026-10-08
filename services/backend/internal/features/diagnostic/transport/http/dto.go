package diagnostichttp

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"

type AudioMetadataResponse struct {
	VariantTaskID string  `json:"variant_task_id"`
	Status        string  `json:"status" enums:"missing,pending,processing,ready,failed,cancelled"`
	AudioURL      *string `json:"audio_url" extensions:"x-nullable"`
}

type CriterionResultResponse struct {
	Key         string `json:"key" binding:"required"`
	Satisfied   bool   `json:"satisfied" binding:"required"`
	Explanation string `json:"explanation" binding:"required"`
}

type TaskResponse struct {
	ID               string   `json:"variant_task_id" minLength:"1" maxLength:"128" binding:"required"`
	SourceTaskID     string   `json:"source_task_id" minLength:"1" maxLength:"128" binding:"required"`
	Role             string   `json:"role" enums:"main,basic" binding:"required"`
	CompetencyID     string   `json:"competency_id" minLength:"1" maxLength:"128" binding:"required"`
	CompetencyName   string   `json:"competency_name" binding:"required"`
	ConstituentID    string   `json:"constituent_id" minLength:"1" maxLength:"128" binding:"required"`
	ConstituentName  string   `json:"constituent_name" binding:"required"`
	OutcomeID        string   `json:"outcome_id" minLength:"1" maxLength:"128" binding:"required"`
	OutcomeName      string   `json:"outcome_name" binding:"required"`
	Question         string   `json:"question" minLength:"1" binding:"required"`
	Options          []string `json:"options" binding:"required"`
	VoiceInstruction string   `json:"voice_instruction" minLength:"1" maxLength:"500" binding:"required"`
}

type ProgressResponse struct {
	Text             string                    `json:"text,omitempty" minLength:"1" binding:"optional"`
	SessionID        string                    `json:"session_id" minLength:"1" maxLength:"128" binding:"required"`
	Status           string                    `json:"status" enums:"active,completed" binding:"required"`
	Completed        int                       `json:"completed_tasks" minimum:"0" binding:"required"`
	Skipped          int                       `json:"skipped_tasks" minimum:"0" binding:"required"`
	Total            int                       `json:"total_tasks" minimum:"1" binding:"required"`
	Current          *TaskResponse             `json:"current,omitempty" binding:"optional"`
	Score            *int                      `json:"score,omitempty" minimum:"0" maximum:"2" binding:"optional"`
	GraderScore      *int                      `json:"grader_score,omitempty" minimum:"0" maximum:"2" binding:"optional"`
	GraderMaxScore   *int                      `json:"grader_max_score,omitempty" minimum:"1" maximum:"2" binding:"optional"`
	Verdict          string                    `json:"verdict,omitempty" enums:"correct,partial,incorrect" binding:"optional"`
	CriterionResults []CriterionResultResponse `json:"criterion_results,omitempty" binding:"optional"`
	Feedback         []string                  `json:"feedback,omitempty" minItems:"3" maxItems:"3" binding:"optional"`
}

type AnswerResponse struct {
	VariantTaskID    string                    `json:"variant_task_id"`
	SourceTaskID     string                    `json:"source_task_id"`
	CompetencyID     string                    `json:"competency_id"`
	OutcomeID        string                    `json:"outcome_id"`
	Role             string                    `json:"role" enums:"main,basic"`
	Task             TaskResponse              `json:"task"`
	TranscriptionID  string                    `json:"transcription_id"`
	Text             string                    `json:"text"`
	GraderScore      int                       `json:"grader_score" minimum:"0" maximum:"2"`
	GraderMaxScore   int                       `json:"grader_max_score" minimum:"1" maximum:"2"`
	Score            int                       `json:"score" minimum:"0" maximum:"2"`
	Verdict          string                    `json:"verdict" enums:"correct,partial,incorrect"`
	CriterionResults []CriterionResultResponse `json:"criterion_results" minItems:"1"`
	Feedback         []string                  `json:"feedback" minItems:"3" maxItems:"3"`
	CreatedAt        int64                     `json:"created_at"`
}

type SkippedCompetencyResponse struct {
	ID   string `json:"competency_id"`
	Name string `json:"competency_name"`
	Code string `json:"code"`
}

type ResultResponse struct {
	SessionID               string                      `json:"session_id"`
	Status                  string                      `json:"status" enums:"completed"`
	VariantID               string                      `json:"variant_id"`
	MapRevision             int64                       `json:"map_revision"`
	IncludedCompetencyCount int                         `json:"included_competency_count"`
	SkippedCompetencies     []SkippedCompetencyResponse `json:"skipped_competencies"`
	CompletedTasks          int                         `json:"completed_tasks"`
	TotalTasks              int                         `json:"total_tasks"`
	DiagnosticScore         int                         `json:"diagnostic_score"`
	MaximumScore            int                         `json:"maximum_score"`
	Answers                 []AnswerResponse            `json:"answers"`
	UntestedBasics          []TaskResponse              `json:"untested_basics"`
}

func progressResponse(value diagnostic.Progress) ProgressResponse {
	result := ProgressResponse{
		Text:      value.Text,
		SessionID: value.SessionID, Status: value.Status, Completed: value.Completed,
		Skipped: value.Skipped, Total: value.Total, Score: value.Score,
		GraderScore: value.GraderScore, GraderMaxScore: value.GraderMaxScore, Verdict: value.Verdict,
		CriterionResults: criterionResults(value.CriterionResults),
		Feedback:         append([]string(nil), value.Feedback...),
	}
	if value.Current != nil {
		current := taskResponse(*value.Current)
		result.Current = &current
	}
	return result
}

func resultResponse(value diagnostic.Result) ResultResponse {
	result := ResultResponse{
		SessionID: value.SessionID, Status: value.Status, VariantID: value.VariantID,
		MapRevision: value.MapRevision, IncludedCompetencyCount: value.IncludedCompetencyCount,
		CompletedTasks: value.CompletedTasks, TotalTasks: value.TotalTasks,
		DiagnosticScore: value.DiagnosticScore, MaximumScore: value.MaximumScore,
		SkippedCompetencies: make([]SkippedCompetencyResponse, 0, len(value.SkippedCompetencies)),
		Answers:             make([]AnswerResponse, 0, len(value.Answers)),
		UntestedBasics:      make([]TaskResponse, 0, len(value.UntestedBasics)),
	}
	for _, skipped := range value.SkippedCompetencies {
		result.SkippedCompetencies = append(result.SkippedCompetencies, SkippedCompetencyResponse{ID: skipped.ID, Name: skipped.Name, Code: skipped.Code})
	}
	for _, answer := range value.Answers {
		result.Answers = append(result.Answers, AnswerResponse{
			VariantTaskID: answer.VariantTaskID, SourceTaskID: answer.SourceTaskID,
			CompetencyID: answer.CompetencyID, OutcomeID: answer.OutcomeID,
			Role: answer.Role, Task: taskResponse(answer.Task),
			TranscriptionID: answer.TranscriptionID, Text: answer.Text,
			GraderScore: answer.GraderScore, GraderMaxScore: answer.GraderMaxScore, Score: answer.Score, Verdict: answer.Verdict,
			CriterionResults: criterionResults(answer.CriterionResults),
			Feedback:         append([]string(nil), answer.Feedback...), CreatedAt: answer.CreatedAt,
		})
	}
	for _, task := range value.UntestedBasics {
		result.UntestedBasics = append(result.UntestedBasics, taskResponse(task))
	}
	return result
}

func criterionResults(values []diagnostic.CriterionResult) []CriterionResultResponse {
	result := make([]CriterionResultResponse, 0, len(values))
	for _, value := range values {
		result = append(result, CriterionResultResponse{Key: value.Key, Satisfied: value.Satisfied, Explanation: value.Explanation})
	}
	return result
}

func taskResponse(value diagnostic.TaskSnapshot) TaskResponse {
	return TaskResponse{
		ID: value.ID, SourceTaskID: value.SourceTaskID, Role: value.Role,
		CompetencyID: value.CompetencyID, CompetencyName: value.CompetencyName,
		ConstituentID: value.ConstituentID, ConstituentName: value.ConstituentName,
		OutcomeID: value.OutcomeID, OutcomeName: value.OutcomeName,
		Question: value.Question, Options: append([]string{}, value.Options...),
		VoiceInstruction: value.VoiceInstruction,
	}
}
