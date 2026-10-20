package application

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

// Study scopes of a topic in «Что доработать».
const (
	StudyReview  = "review"
	StudyFull    = "full"
	StudySkipped = "skipped"
)

// maxContentRunes bounds each educational content excerpt sent to the model.
const maxContentRunes = 400

// feedbackTopic is the constituent of the task; legacy snapshots without one
// fall back to the outcome.
func feedbackTopic(task diagnostic.TaskSnapshot) string {
	if topic := strings.TrimSpace(task.ConstituentName); topic != "" {
		return topic
	}
	return strings.TrimSpace(task.OutcomeName)
}

// topicKey separates same-named constituents of different competencies;
// legacy snapshots without a constituent fall back to the topic name.
func topicKey(a diagnostic.Answer, topic string) string {
	if a.Task.ConstituentID != "" {
		return a.CompetencyID + "\x00" + a.Task.ConstituentID
	}
	return a.CompetencyID + "\x00\x00" + topic
}

// The texts a finding keeps when the model leaves it out or writes it as a task.
func fallbackStrength(topic, outcome string) string {
	if outcome = strings.TrimSpace(outcome); outcome == "" || outcome == topic {
		return "По теме «" + topic + "» дан полный ответ на проверенное задание."
	}
	return "По теме «" + topic + "» дан полный ответ на проверенное задание по результату «" + outcome + "»."
}

func fallbackTopicText(topic, scope string) string {
	switch scope {
	case StudyFull:
		return "Ответы показали, что тему «" + topic + "» полезно изучить полностью, начиная с основных понятий."
	case StudySkipped:
		return "Вопросы по теме «" + topic + "» пропущены, поэтому её освоение пока не проверено."
	default:
		return "Ответы по теме «" + topic + "» показали пробелы; полезно повторение её основных понятий."
	}
}

func topicScope(e FeedbackEvidence) string {
	switch {
	case e.Skipped:
		return StudySkipped
	case e.FullStudy:
		return StudyFull
	default:
		return StudyReview
	}
}

func appendUnique(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func excerpt(text string) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= maxContentRunes {
		return text
	}
	return string([]rune(text)[:maxContentRunes]) + "…"
}

func AnalyzeResult(result diagnostic.Result) DeterministicReport {
	report := DeterministicReport{SessionID: result.SessionID, DiagnosticScore: result.DiagnosticScore,
		MaximumScore: result.MaximumScore, Strengths: []string{}, ConfirmedGaps: []diagnostic.ConfirmedGap{},
		PartialCompetencies:     []diagnostic.PartialCompetency{},
		UnverifiedCompetencies:  append([]diagnostic.SkippedCompetency(nil), result.SkippedCompetencies...),
		TrainingRecommendations: []diagnostic.TrainingRecommendation{}, Evidence: []FeedbackEvidence{}}
	if result.MaximumScore > 0 {
		report.ScorePercentage = result.DiagnosticScore * 100 / result.MaximumScore
	}
	// «Что доработать» speaks about topics: every outcome with a gap joins the
	// evidence of its topic, and the model writes one text per topic.
	topics := map[string]int{}
	for _, finding := range diagnostic.OutcomeFindings(result.Answers) {
		a := finding.Answer
		topic := feedbackTopic(a.Task)
		id := a.Task.OutcomeID
		if id == "" {
			id = a.OutcomeID
		}
		// Use the observed answer and clarification, not the closing recommendation.
		observations := []string{}
		if !a.Skipped {
			observations = append(observations, a.Feedback[:min(2, len(a.Feedback))]...)
		}
		if finding.Mastered {
			report.Evidence = append(report.Evidence, FeedbackEvidence{
				ID: fmt.Sprintf("strength:%d", len(report.Strengths)), Topic: topic,
				Outcomes: []string{a.Task.OutcomeName}, Observations: observations})
			report.Strengths = append(report.Strengths, fallbackStrength(topic, a.Task.OutcomeName))
			continue
		}
		key := topicKey(a, topic)
		index, ok := topics[key]
		if !ok {
			index = len(report.Evidence)
			topics[key] = index
			report.Evidence = append(report.Evidence, FeedbackEvidence{
				ID: fmt.Sprintf("topic:%d", len(topics)-1), Topic: topic,
				Outcomes: []string{}, Observations: []string{}, Skipped: true})
		}
		evidence := &report.Evidence[index]
		evidence.Outcomes = appendUnique(evidence.Outcomes, a.Task.OutcomeName)
		evidence.EducationalContent = appendUnique(evidence.EducationalContent, excerpt(a.Task.EducationalContent))
		evidence.Observations = append(evidence.Observations, observations...)
		evidence.FullStudy = evidence.FullStudy || finding.FullStudy
		evidence.Skipped = evidence.Skipped && a.Skipped
		evidence.targets = append(evidence.targets, outcomeRef{a.CompetencyID, id})
		failed := []string{}
		for _, cr := range a.CriterionResults {
			if !cr.Satisfied && strings.TrimSpace(cr.Explanation) != "" {
				failed = append(failed, cr.Explanation)
				if !a.Skipped {
					evidence.FailedCriteria = appendUnique(evidence.FailedCriteria, cr.Explanation)
				}
			}
		}
		// Keep the existing wire shape: partial main answers use details;
		// zero answers use gaps. The text is set per topic below.
		if a.Role == "main" && a.Score == 1 && !a.Skipped {
			report.PartialCompetencies = append(report.PartialCompetencies, diagnostic.PartialCompetency{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, Topic: topic, TopicID: evidence.ID})
		} else {
			report.ConfirmedGaps = append(report.ConfirmedGaps, diagnostic.ConfirmedGap{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, OutcomeID: id,
				OutcomeName: a.Task.OutcomeName, TaxonomyCode: a.Task.TaxonomyCode, Importance: a.Task.Importance,
				FailedCriteria: failed, Topic: topic, TopicID: evidence.ID})
		}
		report.TrainingRecommendations = append(report.TrainingRecommendations, diagnostic.TrainingRecommendation{
			CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName,
			OutcomeID: id, OutcomeName: a.Task.OutcomeName})
	}
	for _, evidence := range report.Evidence {
		if strings.HasPrefix(evidence.ID, "topic:") {
			setTopicText(&report, evidence, fallbackTopicText(evidence.Topic, topicScope(evidence)))
		}
	}
	// Legacy snapshots keep the outcome ID on the answer, as the items above.
	importance := map[string]int16{}
	for _, a := range result.Answers {
		id := a.Task.OutcomeID
		if id == "" {
			id = a.OutcomeID
		}
		importance[id] = a.Task.Importance
	}
	sort.SliceStable(report.TrainingRecommendations, func(i, j int) bool {
		return importance[report.TrainingRecommendations[i].OutcomeID] > importance[report.TrainingRecommendations[j].OutcomeID]
	})
	for i := range report.TrainingRecommendations {
		report.TrainingRecommendations[i].Priority = i + 1
	}
	return report
}

// setTopicText gives every finding of the topic, and its plan targets, the
// same text and study scope. Findings are matched by topic ID, not by name.
func setTopicText(report *DeterministicReport, evidence FeedbackEvidence, text string) {
	scope := topicScope(evidence)
	for i := range report.ConfirmedGaps {
		if gap := &report.ConfirmedGaps[i]; gap.TopicID == evidence.ID {
			gap.Advice, gap.StudyScope = text, scope
		}
	}
	for i := range report.PartialCompetencies {
		if part := &report.PartialCompetencies[i]; part.TopicID == evidence.ID {
			part.Details, part.StudyScope = text, scope
		}
	}
	for _, target := range evidence.targets {
		for i := range report.TrainingRecommendations {
			if rec := &report.TrainingRecommendations[i]; rec.CompetencyID == target.competencyID && rec.OutcomeID == target.outcomeID {
				rec.Rationale = text
			}
		}
	}
}
