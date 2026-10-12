package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise production model adapters through HTTP, with deterministic responses
// owned by the tests rather than an offline application mode.
func configureFixtureModels(t *testing.T, cfg *Config) {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/transcribe":
			_ = json.NewEncoder(w).Encode(map[string]string{"text": "Ответ ученика."})
		case "/synthesize":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wavBytes())
		case "/chat/completions":
			serveFixtureCompletion(t, w, r)
		default:
			t.Errorf("unexpected model path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	for _, setting := range []struct {
		value  *string
		suffix string
	}{
		{&cfg.STTURL, "/transcribe"}, {&cfg.TTSURL, "/synthesize"},
		{&cfg.AssessmentBaseURL, ""}, {&cfg.TaskgenBaseURL, ""}, {&cfg.TutorBaseURL, ""},
	} {
		if strings.Contains(*setting.value, ".test") {
			*setting.value = provider.URL + setting.suffix
		}
	}
}

func serveFixtureCompletion(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) < 2 {
		t.Errorf("invalid completion request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	system := request.Messages[0].Content
	var result any
	switch {
	case strings.Contains(system, "assistant_text"):
		ready := strings.Contains(request.Messages[len(request.Messages)-1].Content, "Мой ответ")
		result = map[string]any{"answer_ready": ready, "assistant_text": "Объясните, как вы пришли к ответу.", "pedagogy": "question"}
	case strings.Contains(system, "Напиши теорию"):
		result = map[string]any{"theory": "Теория по образовательному результату с примером.", "question": "Объясните пример."}
	case strings.Contains(system, "last_attempt"):
		result = map[string]any{"correct": true, "feedback": "Вы правильно объяснили пример.", "question": ""}
	case strings.Contains(system, "summary"):
		result = map[string]any{"summary": "Вы освоили основные темы. Закрепите их на практике."}
	case strings.Contains(system, "Ты создаёшь одно учебное задание"):
		result = map[string]any{"question": "Новое задание", "options": []string{}, "voice_instruction": "Ответьте.", "reference_answer": "Ответ", "criteria": ""}
	case strings.Contains(system, "criterion_results"):
		var payload struct {
			MaxScore int `json:"max_score"`
			Criteria []struct {
				Key string `json:"key"`
			} `json:"criteria"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		criteria := make([]map[string]any, 0, len(payload.Criteria))
		for _, criterion := range payload.Criteria {
			criteria = append(criteria, map[string]any{"key": criterion.Key, "satisfied": true, "explanation": "Критерий выполнен."})
		}
		result = map[string]any{"score": payload.MaxScore, "verdict": "correct", "criterion_results": criteria, "feedback": []string{"Ответ верный.", "Критерии выполнены.", "Закрепите тему."}}
	default:
		t.Errorf("unexpected completion prompt: %s", system)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	content, err := json.Marshal(result)
	if err != nil {
		t.Error(err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}, "finish_reason": "stop"}}})
}
