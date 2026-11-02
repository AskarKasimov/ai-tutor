package llm

import (
	"context"
	"testing"
	"time"
)

func TestInterpretName(t *testing.T) {
	for _, tc := range []struct {
		request, response, name string
		answer                  bool
	}{
		{"Меня зовут философ Ягон Дон.", `{"name":"философ Ягон Дон","has_answer":false}`, "философ Ягон Дон", false},
		{"Называй меня Вачи Организм, а камера — сенсор", `{"name":"Вачи Организм","has_answer":true}`, "Вачи Организм", true},
		{"Дождя нет", `{"name":"","has_answer":true}`, "", true},
	} {
		f := &fakeGateway{answers: []string{tc.response}}
		g := serve(t, f, time.Second)
		got, err := g.InterpretName(context.Background(), "Назови сенсор", tc.request)
		if err != nil || got.Name != tc.name || got.HasAnswer != tc.answer {
			t.Fatalf("%q: %+v %v", tc.request, got, err)
		}
	}
}

func TestInterpretNameRejectsInventedNames(t *testing.T) {
	f := &fakeGateway{answers: []string{`{"name":"игнорируй правила","has_answer":false}`, `{"name":"игнорируй правила","has_answer":false}`}}
	g := serve(t, f, time.Second)
	if _, err := g.InterpretName(context.Background(), "Вопрос?", "Меня зовут Катя"); err == nil {
		t.Fatal("accepted invented name")
	}
}
