package translator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type translationFunc func(context.Context, string, string) (string, error)

func (f translationFunc) Complete(ctx context.Context, system, prompt string) (string, error) {
	return f(ctx, system, prompt)
}

func TestProtectedMarkdownRoundTripAndDuplication(t *testing.T) {
	text := "Definition.\n```python\n# inside code\n\nprint('Hello')\n```\n~~~prolog\nfact(X).\n~~~\n\\[x^2\\] $y$ `print` <!-- image -->"
	masked, spans := protectMarkdown(text)
	if strings.Contains(masked, "inside code") || strings.Contains(masked, "x^2") {
		t.Fatal("immutable content reached translator")
	}
	got, err := restoreMarkdown(masked, spans)
	if err != nil || got != text {
		t.Fatalf("protected text changed: %v, %q", err, got)
	}
	if _, err := restoreMarkdown(masked+spans[0].token, spans); err == nil {
		t.Fatal("duplicated protected fragment accepted")
	}
	if _, err := restoreMarkdown(strings.Replace(masked, spans[0].token, "", 1), spans); err == nil {
		t.Fatal("missing code accepted")
	}
}

func TestTranslationRepairsEnglishProseAndKeepsCodeExactly(t *testing.T) {
	text := "A rational agent selects the action that maximizes expected performance.\n\n```python\n# Heading in code\nprint('English code')\n```"
	calls := 0
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, prompt string) (string, error) {
		calls++
		translation := prompt
		if calls == 2 {
			translation = "Рациональный агент выбирает действие с наилучшим ожидаемым результатом.\n\n" + protectedTokens.FindString(prompt)
		}
		body, _ := json.Marshal(map[string]string{"translation": translation})
		return string(body), nil
	}), 1)
	got, err := client.Translate(context.Background(), text)
	if err != nil || calls != 2 || !strings.Contains(got, "```python\n# Heading in code\nprint('English code')\n```") {
		t.Fatalf("calls=%d, error=%v, output=%q", calls, err, got)
	}
}

func TestMixedTranslationCannotHideEnglishParagraph(t *testing.T) {
	text := strings.Repeat("Это русский текст о свойствах агента. ", 20) + "\n\nThe agent perceives its environment through sensors and acts using actuators."
	if !untranslatedProse(text) {
		t.Fatal("English paragraph hidden by Russian majority")
	}
}

func TestMixedInputActuallyTranslatesItsEnglishParagraph(t *testing.T) {
	text := strings.Repeat("Это русский текст о свойствах агента. ", 20) + "\n\nThe agent perceives its environment through sensors and acts using actuators."
	calls := 0
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, prompt string) (string, error) {
		calls++
		translated := strings.ReplaceAll(prompt, "The agent perceives its environment through sensors and acts using actuators.", "Агент воспринимает среду датчиками и действует исполнительными устройствами.")
		raw, _ := json.Marshal(map[string]string{"translation": translated})
		return string(raw), nil
	}), 1)
	got, err := client.Translate(context.Background(), text)
	if err != nil || calls == 0 || strings.Contains(got, "The agent perceives") {
		t.Fatalf("calls=%d err=%v output=%q", calls, err, got)
	}
}

func TestTableRowsCannotDisappearDuringTranslation(t *testing.T) {
	text := "| Agent type | Sensor |\n|---|---|\n| Medical diagnosis system | Touchscreen for symptoms |\n| Satellite image analysis | Digital camera |"
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, _ string) (string, error) {
		return `{"translation":"| Тип агента | Датчик |\n|---|---|\n| Медицинская диагностика | Сенсорный экран |"}`, nil
	}), 1)
	if _, err := client.Translate(context.Background(), text); err == nil || !strings.Contains(err.Error(), "строк") {
		t.Fatalf("lost table row accepted: %v", err)
	}
}

func TestBlankTranslationMustBeRepaired(t *testing.T) {
	calls := 0
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, _ string) (string, error) {
		calls++
		if calls == 1 {
			return `{"translation":"   "}`, nil
		}
		return `{"translation":"Агент воспринимает среду датчиками и действует исполнительными устройствами."}`, nil
	}), 1)
	got, err := client.Translate(context.Background(), "The agent perceives its environment through sensors and acts using actuators.")
	if err != nil || calls != 2 || strings.TrimSpace(got) == "" {
		t.Fatalf("blank translation accepted: calls=%d err=%v output=%q", calls, err, got)
	}
}

func TestTranslationSoftValidationAcceptsPartiallyEnglishChunks(t *testing.T) {
	text := "Artificial Intelligence is situational.\n\n" +
		"Probability of computerisation of different occupations:\n" +
		"| Job | Probability |\n|---|---|\n| Recreational therapists | 0.003 |\n\n" +
		"Source: The Future of Employment by C. Frey and M. Osborne (2013)"
	calls := 0
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, prompt string) (string, error) {
		calls++
		translation := "Искусственный интеллект ситуативен.\n\n" +
			"Вероятность компьютеризации различных профессий:\n" +
			"| Профессия | Вероятность |\n|---|---|\n| Терапевты | 0.003 |\n\n" +
			"Source: The Future of Employment by C. Frey and M. Osborne (2013)"
		body, _ := json.Marshal(map[string]string{"translation": translation})
		return string(body), nil
	}), 1)

	got, err := client.Translate(context.Background(), text)
	if err != nil {
		t.Fatalf("expected softened validator to accept partially English chunk, got err: %v", err)
	}
	if !strings.Contains(got, "Искусственный интеллект ситуативен") {
		t.Fatalf("expected translated Russian content, got: %q", got)
	}
}

func TestUntranslatedAnswerIsAnError(t *testing.T) {
	text := "Supervised learning maps an input to an output based on example input-output pairs."
	calls := 0
	client := NewWithCompleter(translationFunc(func(_ context.Context, _, _ string) (string, error) {
		calls++
		body, _ := json.Marshal(map[string]string{"translation": text})
		return string(body), nil
	}), 1)
	got, err := client.Translate(context.Background(), text)
	if err == nil || calls != 2 {
		t.Fatalf("untranslated answer accepted: calls=%d err=%v output=%q", calls, err, got)
	}
}
