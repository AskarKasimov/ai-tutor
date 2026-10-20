package application

import (
	"reflect"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func narrativeReport() DeterministicReport {
	return AnalyzeResult(diagnostic.Result{DiagnosticScore: 1, MaximumScore: 3, Answers: []diagnostic.Answer{
		{CompetencyID: "c", Role: "main", Score: 1, Feedback: []string{"Вы выбрали тип задачи.", "Не объяснили целевую переменную.", "Выберите вариант."}, Task: diagnostic.TaskSnapshot{OutcomeID: "main", OutcomeName: "Выбирает тип задачи", ConstituentID: "p", ConstituentName: "1.3 Постановка задачи", EducationalContent: "Типы задач и целевая переменная."}},
		{CompetencyID: "c", Role: "basic", Score: 1, Task: diagnostic.TaskSnapshot{OutcomeID: "b", OutcomeName: "Различает типы признаков"}},
		{CompetencyID: "c", Role: "basic", Score: 0, Task: diagnostic.TaskSnapshot{OutcomeID: "b2", OutcomeName: "Определяет целевую переменную", ConstituentID: "p", ConstituentName: "1.3 Постановка задачи"}},
	}})
}

func TestGapsOfATopicShareOneEvidence(t *testing.T) {
	report := narrativeReport()
	if len(report.Evidence) != 2 || report.Evidence[0].ID != "topic:0" || report.Evidence[1].ID != "strength:0" {
		t.Fatalf("evidence: %+v", report.Evidence)
	}
	topic := report.Evidence[0]
	if topic.Topic != "1.3 Постановка задачи" || !reflect.DeepEqual(topic.Outcomes, []string{"Выбирает тип задачи", "Определяет целевую переменную"}) ||
		!reflect.DeepEqual(topic.EducationalContent, []string{"Типы задач и целевая переменная."}) {
		t.Fatalf("topic evidence: %+v", topic)
	}
	// The observed answer and clarification, not the closing task action.
	if !reflect.DeepEqual(topic.Observations, []string{"Вы выбрали тип задачи.", "Не объяснили целевую переменную."}) {
		t.Fatalf("observations: %v", topic.Observations)
	}
	if report.PartialCompetencies[0].Topic != topic.Topic || report.ConfirmedGaps[0].Topic != topic.Topic ||
		report.ConfirmedGaps[0].StudyScope != StudyReview || report.ConfirmedGaps[0].Advice != report.PartialCompetencies[0].Details {
		t.Fatalf("findings are not tied to their topic: %+v %+v", report.ConfirmedGaps, report.PartialCompetencies)
	}
}

func TestWrittenFeedbackUsesIDsAndPreservesDiagnosticFacts(t *testing.T) {
	report := narrativeReport()
	before := narrativeReport()
	// Deliberately reordered: prose must follow IDs rather than array positions.
	written := WrittenFeedback{Summary: "Постановка задачи требует внимания.", Details: []WrittenDetail{
		{ID: "topic:0", Text: "Полезно повторить, как тип задачи связан с целевой переменной."},
		{ID: "strength:0", Text: "Вы различаете числовые и категориальные признаки."},
	}}
	if !applyWrittenFeedback(&report, written) {
		t.Fatal("complete prose reported as incomplete")
	}
	topic := written.Details[0].Text
	if report.Summary != written.Summary || report.Strengths[0] != written.Details[1].Text ||
		report.PartialCompetencies[0].Details != topic || report.ConfirmedGaps[0].Advice != topic {
		t.Fatalf("model prose was not applied to its finding: %+v", report)
	}
	for _, rec := range report.TrainingRecommendations {
		if rec.Rationale != topic {
			t.Fatalf("plan rationale: %+v", rec)
		}
	}
	before.Summary = report.Summary
	before.Strengths = report.Strengths
	before.ConfirmedGaps[0].Advice = topic
	before.PartialCompetencies[0].Details = topic
	for i := range before.TrainingRecommendations {
		before.TrainingRecommendations[i].Rationale = topic
	}
	if !reflect.DeepEqual(before, report) {
		t.Fatal("model prose changed diagnostic facts")
	}
}

func TestWrittenFeedbackKeepsFallbackForMissingOrTaskLikeProse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		written WrittenFeedback
	}{
		{"missing item", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}}}},
		{"duplicate id", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}, {ID: "topic:0", Text: "Текст."}, {ID: "topic:0", Text: "Другой."}}}},
		{"empty text", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}, {ID: "topic:0", Text: " "}}}},
		{"question", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}, {ID: "topic:0", Text: "Чем регрессия отличается от классификации?"}}}},
		{"imperative", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}, {ID: "topic:0", Text: "Пересмотрите постановку задачи."}}}},
		{"request", WrittenFeedback{Summary: "Итог.", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст."}, {ID: "topic:0", Text: "Стоит вам добавить целевую переменную."}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := narrativeReport()
			want := report.ConfirmedGaps[0].Advice
			if applyWrittenFeedback(&report, tc.written) {
				t.Fatal("invalid model response reported as complete")
			}
			if report.ConfirmedGaps[0].Advice != want || want != fallbackTopicText("1.3 Постановка задачи", StudyReview) {
				t.Fatalf("fallback replaced: %q", report.ConfirmedGaps[0].Advice)
			}
			if report.Strengths[0] != "Текст." {
				t.Fatal("valid prose dropped with the invalid one")
			}
		})
	}
	report := narrativeReport()
	if applyWrittenFeedback(&report, WrittenFeedback{Summary: "Что дальше?", Details: []WrittenDetail{}}) || report.Summary != "" {
		t.Fatal("question accepted as the summary")
	}
}
