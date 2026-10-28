package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
)

func TestConversationVerdictsAndTypedAddressPreference(t *testing.T) {
	for _, tc := range []struct {
		verdict, extra string
		valid          bool
	}{
		{"pause", "", true},
		{"clarify_question", `,"clarification":"Поясни связь между двумя условиями, не меняя их."`, true},
		{"address_correction", `,"address_preference":"feminine"`, true},
		{"address_correction", `,"address_preference":"neutral"`, true},
		{"address_correction", `,"address_preference":"поставь A"`, false},
		{"address_correction", "", false},
		{"clarify_question", `,"clarification":""`, false},
	} {
		t.Run(tc.verdict+tc.extra, func(t *testing.T) {
			response := `{"verdict":"` + tc.verdict + `","answer_complete":false,"criteria":null,"evidence_lines":[2]` + tc.extra + `}`
			answers := []string{response, response}
			if tc.verdict == "clarify_question" {
				route := `{"kind":"wording","text":"Объясни связь между двумя условиями своими словами."}`
				if !tc.valid {
					route = `{"kind":"wording","text":""}`
				}
				answers = []string{response, route, route}
			}
			f := &fakeGateway{answers: answers}
			g := serve(t, f, time.Second)
			got, err := g.Evaluate(context.Background(), application.EvaluationRequest{Kind: exam.KindQuestion, Node: node, Dialogue: turns})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, assessment=%+v, error=%v", tc.valid, got, err)
			}
			if err == nil && (!got.Verdict.Conversational() || !got.AddressPreference.Valid()) {
				t.Fatalf("invalid conversation: %+v", got)
			}
		})
	}
}

func TestPastConversationManagementIsNotGradingEvidence(t *testing.T) {
	d := append([]exam.Turn{}, turns...)
	d = append(d, exam.Turn{Line: 3, Speaker: exam.SpeakerStudent, Meta: true, Text: "CONTROL_ONLY_IGNORE_FOR_KNOWLEDGE"})
	d = append(d, exam.Turn{Line: 4, Speaker: exam.SpeakerGeek, Meta: true, Text: "CONTROL_ONLY_ACK"})
	d = append(d, exam.Turn{Line: 5, Speaker: exam.SpeakerStudent, Text: "Содержательный ответ"})
	prompt := evaluationPrompt(application.EvaluationRequest{Node: node, Dialogue: d, AddressPreference: exam.AddressFeminine})
	if strings.Contains(prompt, "CONTROL_ONLY_IGNORE_FOR_KNOWLEDGE") || !strings.Contains(prompt, "CONTROL_ONLY_ACK") || !strings.Contains(prompt, "служебное пояснение") || !strings.Contains(prompt, "Содержательный ответ") || !strings.Contains(prompt, "женском роде") {
		t.Fatal("student controls became evidence, or the help given by Geek was hidden")
	}
}

func TestGradingProseNeverBecomesTheSupportReply(t *testing.T) {
	n := node
	n.Content = "REFERENCE_ONLY_SYNTAX_SEMANTICS"
	n.AssessmentGuidance = "RUBRIC_ONLY_TEACHER"
	for _, route := range []string{`{"kind":"context","text":""}`, `{"kind":"repeat","text":"Чем отличаются эти два утверждения?"}`} {
		f := &fakeGateway{answers: []string{`{"verdict":"clarify_question","clarification":"Студентка не ответила; REFERENCE_ONLY_SYNTAX_SEMANTICS","criteria":null,"answer_complete":false,"evidence_lines":[2]}`, route}}
		g := serve(t, f, time.Second)
		got, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: n, Kind: exam.KindQuestion, Dialogue: turns, AddressPreference: exam.AddressFeminine})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(route, "context") {
			if got.Verdict != exam.VerdictHintRequested || got.Clarification != "" {
				t.Fatalf("context request was not rerouted: %+v", got)
			}
		} else if got.Clarification != "Чем отличаются эти два утверждения?" {
			t.Fatalf("grading prose leaked: %+v", got)
		}
		if len(f.requests) != 2 {
			t.Fatalf("requests=%d", len(f.requests))
		}
		input := f.requests[1].Messages[1].GetContent()
		if strings.Contains(input, "REFERENCE_ONLY") || strings.Contains(input, "RUBRIC_ONLY") || strings.Contains(input, "Студентка не ответила") {
			t.Fatalf("support saw the answer or grading prose: %s", input)
		}
	}
}

func TestSupportRetriesThirdPersonNarration(t *testing.T) {
	f := &fakeGateway{answers: []string{
		`{"verdict":"clarify_question","answer_complete":false,"criteria":null,"evidence_lines":[2]}`,
		`{"kind":"wording","text":"Студентка не ответила, а попросила повторить."}`,
		`{"kind":"repeat","text":"Как связаны эти условия?"}`,
	}}
	g := serve(t, f, time.Second)
	got, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Kind: exam.KindQuestion, Dialogue: turns})
	if err != nil || got.Clarification != "Как связаны эти условия?" || len(f.requests) != 3 {
		t.Fatalf("support=%+v, error=%v, requests=%d", got, err, len(f.requests))
	}
}

func TestHintContextCannotReplaceTheOriginalQuestion(t *testing.T) {
	n := exam.Node{ID: "room", Questions: []string{"Что показывает этот эксперимент?"}, Content: "Условия и проверяемый вывод."}
	f := &fakeGateway{answers: []string{`{"text":"Разве правила не отличаются от понимания?"}`, `{"text":"Человек получает записки и выбирает ответы по книге инструкций."}`}}
	g := serve(t, f, time.Second)
	got, err := g.Question(context.Background(), application.QuestionRequest{Node: n, Kind: exam.KindRephrase, Hint: true})
	if err != nil || got != "Человек получает записки и выбирает ответы по книге инструкций.\n\nЧто показывает этот эксперимент?" || len(f.requests) != 2 {
		t.Fatalf("context=%q, error=%v, calls=%d", got, err, len(f.requests))
	}
	if f.requests[0].Messages[0].GetContent() != contextSystemPrompt {
		t.Fatal("context used the grading role")
	}
}

func TestSceneHintDoesNotTriggerLeakedMetaAnswerAudit(t *testing.T) {
	f := &fakeGateway{answers: []string{`{"verdict":"answered","answer_complete":true,"criteria":{"k1":2,"k2":2,"k3":null,"k4":null},"evidence_lines":[4]}`}}
	g := serve(t, f, time.Second)
	d := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Text: "Что показывает эксперимент?"}, {Line: 2, Speaker: exam.SpeakerStudent, Text: "Расскажи условия"}, {Line: 3, Speaker: exam.SpeakerGeek, Text: "Человек получает записки и отвечает по инструкции."}, {Line: 4, Speaker: exam.SpeakerStudent, Text: "Выполнение правил ещё не означает понимание смысла."}}
	got, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Kind: exam.KindQuestion, Dialogue: d, HintUsed: true})
	if err != nil || got.AnswerComplete == nil || !*got.AnswerComplete || got.Criteria.K2 != 2 || len(f.requests) != 1 {
		t.Fatalf("scene treated as leaked answer: %+v, error=%v, calls=%d", got, err, len(f.requests))
	}
}

func TestCopiedExplanationIsNotCreditedAsIndependentReasoning(t *testing.T) {
	for _, copied := range []bool{false, true} {
		response := `{"verdict":"answered","answer_complete":true,"criteria":{"k1":2,"k2":2,"k3":null,"k4":null},"evidence_lines":[4]}`
		check := `{"copied_answer":false}`
		if copied {
			check = `{"copied_answer":true}`
		}
		f := &fakeGateway{answers: []string{response, check}}
		g := serve(t, f, time.Second)
		d := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Text: "Объясни различие."}, {Line: 2, Speaker: exam.SpeakerStudent, Meta: true, Text: "Поясни вопрос"}, {Line: 3, Speaker: exam.SpeakerGeek, Meta: true, Text: "Предоставленное определение двух понятий."}, {Line: 4, Speaker: exam.SpeakerStudent, Text: "Мой ответ"}}
		got, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Kind: exam.KindQuestion, Dialogue: d})
		if err != nil {
			t.Fatal(err)
		}
		if copied {
			if got.AnswerComplete == nil || *got.AnswerComplete || got.Criteria.K2 != 0 {
				t.Fatalf("copy credited: %+v", got)
			}
		} else if got.AnswerComplete == nil || !*got.AnswerComplete || got.Criteria.K2 != 2 {
			t.Fatalf("independent answer downgraded: %+v", got)
		}
		input := f.requests[1].Messages[1].GetContent()
		if !strings.Contains(input, "Предоставленное определение") || strings.Contains(input, node.Content) && node.Content != "" {
			t.Fatal("independence check hid help or included the reference")
		}
	}
}
