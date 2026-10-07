package assessment

import (
	"encoding/json"
	"testing"
)

func TestEvaluationJSONRequiresFieldsButAcceptsValidZeroAndFalse(t *testing.T) {
	const valid = `{"score":0,"verdict":"incorrect","criterion_results":[{"key":"correctness","satisfied":false,"explanation":"Нет правильного ответа."}],"feedback":["Неверно.","Ответ не совпал.","Повторите тему."]}`
	var got Evaluation
	if err := json.Unmarshal([]byte(valid), &got); err != nil {
		t.Fatalf("valid zero/false response rejected: %v", err)
	}
	if got.Score != 0 || got.CriterionResults[0].Satisfied {
		t.Fatalf("zero values changed during decoding: %+v", got)
	}
}

func TestEvaluationJSONRejectsMissingNullAndUnknownFields(t *testing.T) {
	cases := map[string]string{
		"missing score":       `{"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":false,"explanation":"x"}],"feedback":["a","b","c"]}`,
		"null score":          `{"score":null,"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":false,"explanation":"x"}],"feedback":["a","b","c"]}`,
		"null satisfied":      `{"score":0,"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":null,"explanation":"x"}],"feedback":["a","b","c"]}`,
		"missing explanation": `{"score":0,"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":false}],"feedback":["a","b","c"]}`,
		"unknown field":       `{"score":0,"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":false,"explanation":"x"}],"feedback":["a","b","c"],"extra":true}`,
		"trailing value":      `{"score":0,"verdict":"incorrect","criterion_results":[{"key":"k","satisfied":false,"explanation":"x"}],"feedback":["a","b","c"]}{}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			var got Evaluation
			if err := json.Unmarshal([]byte(input), &got); err == nil {
				t.Fatal("expected invalid contract response to be rejected")
			}
		})
	}
}
