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
