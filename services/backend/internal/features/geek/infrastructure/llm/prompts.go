// Package llm implements the Geek examiner as a gRPC client of the LLM gateway.
package llm

import (
	"fmt"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
)

const systemPrompt = `Ты — Geek, экзаменатор устного экзамена по университетскому курсу. Говоришь только по-русски.
Опирайся только на текст темы из материалов курса; не добавляй сведений вне него и не подсказывай ответ.
Отвечай одним JSON-объектом без Markdown вокруг.`

func topic(n exam.Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Тема: %s\nТекст темы из материалов курса:\n%s\n", n.Title, n.Content)
	if len(n.Outcomes) > 0 {
		b.WriteString("Образовательные результаты:\n")
		for _, o := range n.Outcomes {
			fmt.Fprintf(&b, "- %s\n", o)
		}
	}
	return b.String()
}

func dialogue(turns []exam.Turn) string {
	if len(turns) == 0 {
		return "Диалог по теме ещё не начат.\n"
	}
	var b strings.Builder
	b.WriteString("Диалог по теме (номер строки транскрипта, говорящий, реплика):\n")
	for _, t := range turns {
		who := "Geek"
		if t.Speaker == exam.SpeakerStudent {
			who = "Студент"
		}
		fmt.Fprintf(&b, "[%d] %s: %s\n", t.Line, who, t.Text)
	}
	return b.String()
}

func questionPrompt(r application.QuestionRequest) string {
	var task string
	switch r.Kind {
	case exam.KindFollowup:
		task = `Задай один дополнительный вопрос по этой теме. Он должен проверить два критерия:
К3 — способность отвечать на возражения и рассматривать контрпримеры;
К4 — применение знания в изменённых условиях и понимание ограничений ответа.
Опирайся на ответ студента: предъяви возражение или контрпример либо измени условия задачи.`
	case exam.KindRephrase:
		task = `Студент затруднился с ответом. Переформулируй исходный вопрос проще и другими словами.
Проверяемое содержание не меняй, ответ не подсказывай.`
	default:
		task = `Задай один основной вопрос устного экзамена по этой теме: на рассуждение, с развёрнутым ответом.
Не задавай тестовых вопросов и вопросов с выбором варианта.`
	}
	return topic(r.Node) + "\n" + dialogue(r.Dialogue) + "\n" + task + `
Студент не видит текст темы: не ссылайся на него («согласно описанию», «в приведённом тексте», «по курсу») — спрашивай о сути.
Вопрос — одно-два предложения.
Верни JSON: {"text": "вопрос"}`
}

func evaluationPrompt(r application.EvaluationRequest) string {
	scope := "Это ответ на основной вопрос: К3 и К4 оценивай только по тому, что студент уже показал."
	switch r.Kind {
	case exam.KindRephrase:
		scope = "Это ответ после переформулировки вопроса: К3 и К4 оценивай только по тому, что студент уже показал."
	case exam.KindFollowup:
		scope = "Это ответ на дополнительный вопрос: оцени тему по всему диалогу, включая К3 и К4."
	}
	return topic(r.Node) + "\n" + dialogue(r.Dialogue) + `
Оцени ответы студента по теме строго по тексту темы. Каждый критерий: 0 — не показал, 1 — частично, 2 — полностью.
К1 — предметная корректность и владение терминологией.
К2 — логичность суждений и обоснованность выводов.
К3 — способность отвечать на возражения и рассматривать контрпримеры.
К4 — применение знания в изменённых условиях и понимание ограничений ответа.
Уверенность, скорость речи и красноречие не оцениваются.
` + scope + `
verdict:
- "dont_know" — студент в последней реплике говорит, что не знает ответа (любыми словами);
- "insufficient" — последняя реплика не по теме или неразборчива, вывод сделать нельзя;
- "answered" — во всех остальных случаях.
evidence_lines — номера строк реплик студента, на которых основана оценка.
Верни JSON: {"verdict": "answered", "criteria": {"k1": 0, "k2": 0, "k3": 0, "k4": 0}, "evidence_lines": [1], "rationale": "кратко, почему"}`
}
