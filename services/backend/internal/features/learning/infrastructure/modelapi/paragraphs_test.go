package modelapi

import "testing"

func TestTheoryParagraphEscapesPreserveCodeAndMath(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`Первый.\n\nВторой.`, "Первый.\n\nВторой."},
		{`Первый.\r\n\r\nВторой.`, "Первый.\n\nВторой."},
		{"Первый.\n\nВторой.", "Первый.\n\nВторой."},
		{"Команда `print(\"\\n\\n\")` и \\nu + \\nabla f.", "Команда `print(\"\\n\\n\")` и \\nu + \\nabla f."},
		{"```python\nprint(\"\\n\\n\")\n```", "```python\nprint(\"\\n\\n\")\n```"},
	} {
		if got := normalizeTheoryParagraphs(tc.input); got != tc.want {
			t.Fatalf("input=%q got=%q want=%q", tc.input, got, tc.want)
		}
	}
}

func TestRussianProseIgnoresCodeAndFormulas(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"Знак коэффициента показывает направление влияния признака.", true},
		{"Модель `LinearRegression` из scikit-learn: $\\hat y = \\beta_0 + \\beta_1 x$, где $\\beta_1$ — наклон.", true},
		{"Пример:\n\n```python\nmodel = LinearRegression().fit(features, target)\n```", true},
		{"The sign of a coefficient shows the direction of influence: **positive** → increase.", false},
		{"$x$", false},
	} {
		if got := russianProse(tc.text); got != tc.want {
			t.Fatalf("russianProse(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
