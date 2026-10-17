package translator_test

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedLang  string
		expectedNeeds bool
	}{
		{
			name: "English lecture with slide headers and LaTeX formulas",
			input: `# Introduction to Search Algorithms

## Слайд 1

In this lecture we define state space search. The search problem consists of:
- A state space $S$
- An initial state $s_0 \in S$
- A set of actions $A(s)$ for each state $s$
- A transition model $\text{Result}(s, a)$
- A goal test function

## Слайд 2

Breadth-first search expands the shallowest nodes first using a FIFO queue.
The time complexity is $O(b^d)$ where $b$ is the branching factor.
`,
			expectedLang:  "en",
			expectedNeeds: true,
		},
		{
			name: "Russian lecture with AI terminology and math",
			input: `# Лекция 4: Поиск в пространстве состояний

## Слайд 1

В этой лекции мы рассмотрим алгоритмы поиска в пространстве состояний.
Задача поиска включает:
- Множество состояний $S$
- Начальное состояние $s_0 \in S$
- Множество действий $A(s)$
- Переходную модель $\text{Result}(s, a)$
- Проверку достижения цели (Goal test)

Мы используем библиотеки PyTorch и NumPy для численных расчетов.
`,
			expectedLang:  "ru",
			expectedNeeds: false,
		},
		{
			name: "Russian text with Python code and English variable names",
			input: `## Слайд 3

Реализация функции потерь на Python:
` + "```python\ndef compute_loss(predictions, targets):\n    return ((predictions - targets) ** 2).mean()\n```" + `
Здесь вычисляется среднеквадратичная ошибка (MSE) по всем образцам батча.
`,
			expectedLang:  "ru",
			expectedNeeds: false,
		},
		{
			name:          "Empty input",
			input:         "",
			expectedLang:  "unknown",
			expectedNeeds: false,
		},
		{
			name:          "Only code and math",
			input:         "```python\nx = 1\ny = 2\n```\n$$\\int_0^1 x^2 dx$$",
			expectedLang:  "unknown",
			expectedNeeds: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lang, needs := translator.DetectLanguage(tc.input)
			if lang != tc.expectedLang {
				t.Errorf("expected lang %q, got %q", tc.expectedLang, lang)
			}
			if needs != tc.expectedNeeds {
				t.Errorf("expected needs %v, got %v", tc.expectedNeeds, needs)
			}
			if translator.NeedsTranslation(tc.input) != tc.expectedNeeds {
				t.Errorf("NeedsTranslation mismatch, expected %v", tc.expectedNeeds)
			}
		})
	}
}
