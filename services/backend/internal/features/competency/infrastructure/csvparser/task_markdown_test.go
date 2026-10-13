package csvparser

import (
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownTaskBlocks(t *testing.T) {
	question := "Вопрос.\n\n```python\nprint(\"a\\nb\")\n```"
	options := "- **Объект:** автомобиль; Таргет: цена\n  продолжение\n- Срок: 2 - 3 дня\n\n  Второй абзац."
	raw := strings.Join([]string{question, options, "Назовите вариант.", "Цена."}, "\n\n\n")
	for _, input := range []string{raw, strings.ReplaceAll(raw, "\n", "\r\n")} {
		task, ok := parseMLTask(input)
		if !ok {
			t.Fatal("complete Markdown task rejected")
		}
		if task.Question != question || task.VoiceInstruction != "Назовите вариант." || task.ReferenceAnswer != "Цена." {
			t.Fatalf("fields changed: %#v", task)
		}
		want := []string{"**Объект:** автомобиль; Таргет: цена\nпродолжение", "Срок: 2 - 3 дня\n\nВторой абзац."}
		if !reflect.DeepEqual(task.Options, want) {
			t.Fatalf("options = %#v, want %#v", task.Options, want)
		}
	}
}
func TestMarkdownTaskWithoutOptions(t *testing.T) {
	task, ok := parseMLTask("Вопрос\n\n\n[]\n\n\nОбъясните.\n\n\nЭталон")
	if !ok || len(task.Options) != 0 {
		t.Fatalf("open task = %#v, %v", task, ok)
	}
}
func TestMarkdownTaskRejectsMalformedBlocks(t *testing.T) {
	for _, raw := range []string{
		"Вопрос\n\n[]\n\nГолос\n\nОтвет",
		"Вопрос\n\n\nтекст\n\n\nГолос\n\n\nОтвет",
		"Вопрос\n\n\n- \n\n\nГолос\n\n\nОтвет",
		"Вопрос\n\n\n- вариант\n\nвне списка\n\n\nГолос\n\n\nОтвет",
		"Вопрос\n\n\n[]\n\n\n \n\n\nОтвет",
		"Вопрос\n\n\n[]\n\n\nГолос\n\n\nОтвет\n\n\nлишнее",
	} {
		if _, ok := parseMLTask(raw); ok {
			t.Errorf("accepted malformed task: %q", raw)
		}
	}
}

func TestMarkdownOptionsPreserveNestedContent(t *testing.T) {
	raw := "- Выберите код:\n\n  ```python\n  for x in xs:\n      print(\"a\\nb\")\n  ```\n\n  - вложенный пункт\n- **Другой** вариант"
	got, ok := parseMarkdownOptions(raw)
	want := []string{"Выберите код:\n\n```python\nfor x in xs:\n    print(\"a\\nb\")\n```\n\n- вложенный пункт", "**Другой** вариант"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, %v; want %#v", got, ok, want)
	}
}

func TestMarkdownOptionsRequireTextOnMarkerLine(t *testing.T) {
	if _, ok := parseMarkdownOptions("- первый\n-\n  второй"); ok {
		t.Fatal("accepted item with text on a separate line")
	}
}

func TestMarkdownNumberedOptions(t *testing.T) {
	for _, raw := range []string{
		"1. **Первый:** a; b\n   продолжение\n2. Второй",
		"1) **Первый:** a; b\n   продолжение\n2) Второй",
		"1. **Первый:** a; b\n   продолжение\n1. Второй",
	} {
		task, ok := parseMLTask(strings.Join([]string{"Вопрос", raw, "Объясните", "Эталон"}, taskBlockSeparator))
		want := []string{"**Первый:** a; b\nпродолжение", "Второй"}
		if !ok || !reflect.DeepEqual(task.Options, want) {
			t.Fatalf("numbered options = %#v, %v; want %#v", task.Options, ok, want)
		}
	}
}
func TestMarkdownNumberedOptionsPreserveCode(t *testing.T) {
	got, ok := parseMarkdownOptions("1. Код:\n\n   ```python\n   for x in xs:\n       print(x)\n   ```\n2. Другой вариант")
	want := []string{"Код:\n\n```python\nfor x in xs:\n    print(x)\n```", "Другой вариант"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, %v; want %#v", got, ok, want)
	}
}
