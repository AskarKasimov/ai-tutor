package coursegraph

import (
	"encoding/json"
	"fmt"
)

// DefaultAssessmentGuidance is an explicit fallback, not an invented reference
// answer. Only absent criteria use it; a malformed authored scale must fail.
const DefaultAssessmentGuidance = `Общие критерии: в карте для этого задания критерии не заполнены.
Оцени только исходный вопрос и реально сказанное студентом. Объяснение, пример, расчёт или сравнение обязательны только если их явно попросили. Принимай верные краткие ответы своими словами и разумные примеры вне теории. Не требуй пересказа всей темы, точного совпадения слов или дополнительных сведений. Повторение вопроса без нового смысла не является ответом.
Аналитическая рубрика (макс. 4):
Пункт рубрики 1 «Предметная корректность»: 2 — дан конкретный верный ответ или верная часть ответа без существенных ошибок; 1 — есть конкретная верная мысль, но есть и существенная ошибка; 0 — нет конкретного верного содержания по вопросу.
Пункт рубрики 2 «Покрытие вопроса»: 2 — верно раскрыты все явно запрошенные части; 1 — верно раскрыта хотя бы одна часть, но другие пропущены или неверны; 0 — ни одна запрошенная часть не раскрыта верно. Обоснование и пример не добавляй как требования, если их не было в вопросе.`

// Record only selected tasks: outcomes omitted by the ordinary three-question
// selection do not affect this exam. Root metadata survives question preparation.
func withAssessmentWarnings(graph json.RawMessage, outcomes []mapOutcome) (json.RawMessage, error) {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(graph, &shape); err != nil {
		return nil, err
	}
	var topics []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(shape["topics"], &topics); err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, topic := range topics {
		selected[topic.ID] = true
	}
	warnings := []MapIssue{}
	for _, outcome := range outcomes {
		if !selected[topicID(outcome)] {
			continue
		}
		for _, task := range outcome.tasks {
			if task.criteria == "" {
				warnings = append(warnings, MapIssue{Row: outcome.row, Column: fmt.Sprintf("Задание %d", task.position), Message: "Критерии не заданы: используются общие критерии корректности и покрытия вопроса."})
			}
		}
	}
	if len(warnings) == 0 {
		return graph, nil
	}
	data, err := json.Marshal(warnings)
	if err != nil {
		return nil, err
	}
	shape["assessment_warnings"] = data
	return json.Marshal(shape)
}
