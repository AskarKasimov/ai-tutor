package application

import (
	"reflect"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func narrativeReport() DeterministicReport {
	return AnalyzeResult(diagnostic.Result{DiagnosticScore: 1, MaximumScore: 3, Answers: []diagnostic.Answer{
		{CompetencyID: "c", Role: "main", Score: 1, Feedback: []string{"Вы выбрали тип задачи.", "Не объяснили целевую переменную.", "Выберите вариант."}, Task: diagnostic.TaskSnapshot{OutcomeID: "main", OutcomeName: "Выбирает тип задачи", ConstituentID: "p", ConstituentName: "1.3 Постановка задачи"}},
		{CompetencyID: "c", Role: "basic", Score: 1, Task: diagnostic.TaskSnapshot{OutcomeID: "b", OutcomeName: "Различает типы признаков"}},
		{CompetencyID: "c", Role: "basic", Score: 0, Task: diagnostic.TaskSnapshot{OutcomeID: "b2", OutcomeName: "Определяет целевую переменную", ConstituentID: "p", ConstituentName: "1.3 Постановка задачи"}},
	}})
}

func TestWrittenFeedbackUsesIDsAndPreservesDiagnosticFacts(t *testing.T) {
	report := narrativeReport()
	before := narrativeReport()
	// Deliberately reordered: prose must follow IDs rather than array positions.
	written := WrittenFeedback{Summary: "Разберите постановку задачи.", Details: []WrittenDetail{
		{ID: "gap:0", Text: "Повторите, какую величину должна предсказывать модель."},
		{ID: "strength:0", Text: "Вы различаете числовые и категориальные признаки."},
		{ID: "partial:0", Text: "Вы определили тип задачи, но пока не связали его с целевой переменной. Повторите эту связь."},
	}}
	if err := applyWrittenFeedback(&report, written); err != nil {
		t.Fatal(err)
	}
	if report.Strengths[0] != written.Details[1].Text || report.PartialCompetencies[0].Details != "Дополнительно изучить. Составляющая «1.3 Постановка задачи», "+written.Details[2].Text || report.ConfirmedGaps[0].Advice != "Дополнительно изучить. Составляющая «1.3 Постановка задачи», "+written.Details[0].Text {
		t.Fatalf("model prose was not applied to its finding: %+v", report)
	}
	for i := range before.ConfirmedGaps {
		before.ConfirmedGaps[i].Advice = report.ConfirmedGaps[i].Advice
	}
	for i := range before.PartialCompetencies {
		before.PartialCompetencies[i].Details = report.PartialCompetencies[i].Details
	}
	before.Strengths = report.Strengths
	for i := range before.TrainingRecommendations {
		before.TrainingRecommendations[i].Rationale = report.TrainingRecommendations[i].Rationale
	}
	if !reflect.DeepEqual(before, report) {
		t.Fatal("model prose changed diagnostic facts")
	}
	if got := report.Evidence[0].Observations; len(got) != 2 || got[1] != "Не объяснили целевую переменную." {
		t.Fatalf("missing grader evidence or leaked task action: %v", got)
	}
}

func TestWrittenFeedbackRejectsIncompleteOrInventedFindings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		written WrittenFeedback
	}{
		{"missing item", WrittenFeedback{Summary: "Итог", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст"}}}},
		{"unknown id", WrittenFeedback{Summary: "Итог", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст"}, {ID: "gap:0", Text: "Текст"}, {ID: "invented", Text: "Текст"}}}},
		{"duplicate id", WrittenFeedback{Summary: "Итог", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст"}, {ID: "gap:0", Text: "Текст"}, {ID: "gap:0", Text: "Текст"}}}},
		{"empty text", WrittenFeedback{Summary: "Итог", Details: []WrittenDetail{{ID: "strength:0", Text: "Текст"}, {ID: "gap:0", Text: " "}, {ID: "partial:0", Text: "Текст"}}}},
		{"empty summary", WrittenFeedback{Details: []WrittenDetail{{ID: "strength:0", Text: "Текст"}, {ID: "gap:0", Text: "Текст"}, {ID: "partial:0", Text: "Текст"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := narrativeReport()
			if err := applyWrittenFeedback(&report, tc.written); err == nil {
				t.Fatal("invalid model response accepted")
			}
		})
	}
}
