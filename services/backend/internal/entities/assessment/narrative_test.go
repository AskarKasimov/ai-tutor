package assessment

import "testing"

func TestNarrativeKeepsDescriptiveAdvice(t *testing.T) {
	for _, text := range []string{
		"Полезно повторить различие классификации и регрессии.",
		"Внимания требует выбор метрики: `accuracy` не подходит при дисбалансе.",
		"Стоит вернуться к понятию целевой переменной.",
		"Стоит отметить, что тип задачи определён верно.",
		"Не указана причина выбора метрики.",
		"Тип задачи определён верно. Ответ соответствует эталону.",
		"Требует внимания различие обучения с учителем и без учителя.",
	} {
		if !Narrative(text) {
			t.Errorf("descriptive advice rejected: %s", text)
		}
	}
}

// Requests seen in model output and in review: the check must not depend on
// one verb list or on the request verb standing right after the modal word.
func TestNarrativeRejectsQuestionsAndRequests(t *testing.T) {
	for _, text := range []string{
		"Чем регрессия отличается от классификации?",
		"Пересмотрите постановку задачи.",
		"Поясните выбор метрики.",
		"Рассмотрите пример классификации.",
		"Объясни выбор метрики.",
		"Вам нужно сначала объяснить выбор.",
		"Стоит вам добавить целевую переменную.",
		"Ответ верный. Обоснуйте выбор признаков.",
		"Тип задачи определён верно. Ответ соответствует эталону. Почему это классификация? Объясните выбор.",
		"Следует подробнее рассмотреть метрики.",
		"Вам стоит сначала описать данные.",
	} {
		if Narrative(text) {
			t.Errorf("request accepted as narrative: %s", text)
		}
	}
}
