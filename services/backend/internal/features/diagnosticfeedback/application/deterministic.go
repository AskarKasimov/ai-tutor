package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

type DeterministicReport struct {
	SessionID               string
	DiagnosticScore         int
	MaximumScore            int
	ScorePercentage         int
	Strengths               []string
	ConfirmedGaps           []diagnostic.ConfirmedGap
	PartialCompetencies     []diagnostic.PartialCompetency
	UnverifiedCompetencies  []diagnostic.SkippedCompetency
	TrainingRecommendations []diagnostic.TrainingRecommendation
}

var bloomOrder = map[string]int{
	"knowledge":     1,
	"знание":        1,
	"understanding": 2,
	"понимание":     2,
	"application":   3,
	"применение":    3,
	"analysis":      4,
	"анализ":        4,
}

func AnalyzeResult(result diagnostic.Result) DeterministicReport {
	report := DeterministicReport{
		SessionID:               result.SessionID,
		DiagnosticScore:         result.DiagnosticScore,
		MaximumScore:            result.MaximumScore,
		Strengths:               make([]string, 0),
		ConfirmedGaps:           make([]diagnostic.ConfirmedGap, 0),
		PartialCompetencies:     make([]diagnostic.PartialCompetency, 0),
		UnverifiedCompetencies:  append([]diagnostic.SkippedCompetency(nil), result.SkippedCompetencies...),
		TrainingRecommendations: make([]diagnostic.TrainingRecommendation, 0),
	}

	if result.MaximumScore > 0 {
		report.ScorePercentage = (result.DiagnosticScore * 100) / result.MaximumScore
	}

	type compAnswers struct {
		main   *diagnostic.Answer
		basics []diagnostic.Answer
	}
	classified := map[string]string{}
	for _, target := range diagnostic.TrainingTargets(result.Answers) {
		classified[target.Answer.CompetencyID+"\x00"+target.Answer.Task.OutcomeID] = target.Kind
	}

	groups := make(map[string]*compAnswers)
	compOrder := make([]string, 0)

	for i := range result.Answers {
		ans := result.Answers[i]
		grp, ok := groups[ans.CompetencyID]
		if !ok {
			grp = &compAnswers{}
			groups[ans.CompetencyID] = grp
			compOrder = append(compOrder, ans.CompetencyID)
		}
		if ans.Role == "main" {
			grp.main = &ans
		} else if ans.Role == "basic" {
			grp.basics = append(grp.basics, ans)
		}
	}

	for _, compID := range compOrder {
		grp := groups[compID]
		if grp.main == nil {
			continue
		}
		mainAns := grp.main
		compName := mainAns.Task.CompetencyName

		if mainAns.Score == 2 {
			report.Strengths = append(report.Strengths,
				fmt.Sprintf("Компетенция «%s»: высокий уровень владения (%s).", compName, mainAns.Task.OutcomeName))
			continue
		}

		hasBasicZero := false
		for _, b := range grp.basics {
			if classified[b.CompetencyID+"\x00"+b.Task.OutcomeID] == "confirmed_gap" {
				hasBasicZero = true
				failedCriteria := make([]string, 0)
				for _, cr := range b.CriterionResults {
					if !cr.Satisfied && strings.TrimSpace(cr.Explanation) != "" {
						failedCriteria = append(failedCriteria, cr.Explanation)
					}
				}

				advice := ""
				if len(b.Feedback) >= 3 && strings.TrimSpace(b.Feedback[2]) != "" {
					advice = b.Feedback[2]
				}

				report.ConfirmedGaps = append(report.ConfirmedGaps, diagnostic.ConfirmedGap{
					CompetencyID:   compID,
					CompetencyName: compName,
					OutcomeID:      b.OutcomeID,
					OutcomeName:    b.Task.OutcomeName,
					TaxonomyCode:   b.Task.TaxonomyCode,
					Importance:     b.Task.Importance,
					FailedCriteria: failedCriteria,
					Advice:         advice,
				})
			}
		}

		if !hasBasicZero && classified[mainAns.CompetencyID+"\x00"+mainAns.Task.OutcomeID] == "partial_competency" {
			detail := fmt.Sprintf("Базовые понятия темы «%s» усвоены, но требуется закрепление на более высоком уровне (%s).",
				compName, mainAns.Task.TaxonomyCode)
			if len(mainAns.Feedback) >= 2 && strings.TrimSpace(mainAns.Feedback[1]) != "" {
				detail += " Причина: " + mainAns.Feedback[1]
			}
			report.PartialCompetencies = append(report.PartialCompetencies, diagnostic.PartialCompetency{
				CompetencyID:   compID,
				CompetencyName: compName,
				Details:        detail,
			})
		}
	}

	sortGaps := append([]diagnostic.ConfirmedGap(nil), report.ConfirmedGaps...)
	sort.Slice(sortGaps, func(i, j int) bool {
		if sortGaps[i].Importance != sortGaps[j].Importance {
			return sortGaps[i].Importance > sortGaps[j].Importance
		}
		rankI := bloomOrder[strings.ToLower(strings.TrimSpace(sortGaps[i].TaxonomyCode))]
		rankJ := bloomOrder[strings.ToLower(strings.TrimSpace(sortGaps[j].TaxonomyCode))]
		if rankI != rankJ {
			return rankI < rankJ
		}
		if sortGaps[i].CompetencyName != sortGaps[j].CompetencyName {
			return sortGaps[i].CompetencyName < sortGaps[j].CompetencyName
		}
		return sortGaps[i].OutcomeName < sortGaps[j].OutcomeName
	})

	for i, gap := range sortGaps {
		rationale := fmt.Sprintf("Подтверждён пробел в базовом результате «%s».", gap.OutcomeName)
		if gap.Advice != "" {
			rationale += " Рекомендация: " + gap.Advice
		}
		report.TrainingRecommendations = append(report.TrainingRecommendations, diagnostic.TrainingRecommendation{
			CompetencyID:   gap.CompetencyID,
			CompetencyName: gap.CompetencyName,
			OutcomeID:      gap.OutcomeID,
			OutcomeName:    gap.OutcomeName,
			Priority:       i + 1,
			Rationale:      rationale,
		})
	}

	return report
}

type AnswerFeedbackItem struct {
	TaskID             string   `json:"task_id"`
	Topic              string   `json:"topic,omitempty"`
	OutcomeName        string   `json:"outcome_name,omitempty"`
	EducationalContent string   `json:"educational_content,omitempty"`
	Question           string   `json:"question"`
	Transcript         string   `json:"transcript"`
	Score              int      `json:"score"`
	MaxScore           int      `json:"max_score"`
	Feedback           []string `json:"feedback"`
}

type SessionFeedbackReport struct {
	Score           int                  `json:"score"`
	MaxScore        int                  `json:"max_score"`
	ScorePercentage int                  `json:"score_percentage"`
	Strengths       []string             `json:"strengths"`
	Gaps            []string             `json:"gaps"`
	Partials        []string             `json:"partials"`
	Recommendations []string             `json:"recommendations"`
	Items           []AnswerFeedbackItem `json:"items"`
}

var topicCurriculum = map[string][]string{
	"Кластеризация (типы задач ML)": {
		"Обучение без учителя: выявление скрытых структур в данных без известных меток",
		"Сегментация и группировка объектов по сходству признаков (K-Means, DBSCAN)",
		"Отличие кластеризации от классификации и регрессии",
	},
	"Ранжирование (типы задач ML)": {
		"Постановка задачи ранжирования (поисковая выдача, рекомендательные списки)",
		"Упорядочивание объектов по убыванию степени релевантности",
		"Отличие ранжирования от поэлементной классификации",
	},
	"Классификация (типы задач ML)": {
		"Обучение с учителем и дискретная целевая переменная (метки классов)",
		"Бинарная и многоклассовая классификация (one-vs-rest, softmax)",
		"Критерии разделения объектов на классы",
	},
	"Регрессия (типы задач ML)": {
		"Обучение с учителем и непрерывная числовая целевая переменная",
		"Прогнозирование количественных величин (цена, спрос, объём)",
		"Функции потерь регрессии (MSE, MAE, Huber loss)",
	},
	"Диагностика переобучения (Overfitting / Underfitting)": {
		"Анализ кривых обучения: динамика ошибок на train и validation",
		"Признаки переобучения: высокая точность на обучении и падение качества на новых данных",
		"Методы борьбы: регуляризация весов, ранняя остановка, упрощение архитектуры",
	},
	"Схема валидации данных (Train / Val / Test)": {
		"Назначение обучающей (train), валидационной (val) и независимой тестовой (test) выборок",
		"Оценка обобщающей способности модели на отложенных данных",
		"Принципы кросс-валидации и стратификация по классам",
	},
	"Метрики качества при дисбалансе классов (Precision / Recall / F1)": {
		"Неинформативность Accuracy при сильном дисбалансе классов",
		"Матрица ошибок (Confusion Matrix: TP, FP, FN, TN)",
		"Баланс между точностью (Precision), полнотой (Recall) и F1-мерой",
	},
	"Утечка целевой переменной (Data Leakage)": {
		"Признаки, недоступные на момент реального прогноза (признаки из будущего)",
		"Утечки при предобработке данных до разделения выборки на train/test",
		"Изоляция шагов масштабирования и кодирования внутри Pipeline",
	},
	"Аналитическое решение МНК и вырожденность матрицы": {
		"Нормальное уравнение линейной регрессии w = (XᵀX)⁻¹Xᵀy",
		"Условия необратимости матрицы XᵀX (мультиколлинеарность, линейно зависимые признаки)",
		"Случаи неприменимости аналитического решения при большом числе признаков",
	},
	"Интерпретация коэффициентов линейной модели": {
		"Смысл знака коэффициента (положительное/отрицательное влияние на прогноз)",
		"Зависимость весов от масштаба признаков и необходимость стандартизации",
		"Влияние коррелированных признаков на интерпретируемость модели",
	},
	"Градиентный спуск для линейной регрессии на NumPy": {
		"Итерационный шаг градиентного спуска и подбор learning rate",
		"Векторизованное вычисление ошибки и градиента MSE на NumPy",
		"Масштабирование признаков для устойчивой сходимости оптимизатора",
	},
	"L1 и L2 регуляризация весов": {
		"Штраф за величину весов в функции потерь (норма L1 vs L2)",
		"Зануление весов и отбор признаков в Lasso против пропорционального сжатия в Ridge",
		"Подбор коэффициента регуляризации для контроля переобучения",
	},
}

func resolveTopicSubtopics(topic string) []string {
	if subs, ok := topicCurriculum[topic]; ok {
		return subs
	}
	low := strings.ToLower(topic)
	switch {
	case strings.Contains(low, "кластериз"):
		return topicCurriculum["Кластеризация (типы задач ML)"]
	case strings.Contains(low, "ранжиров"):
		return topicCurriculum["Ранжирование (типы задач ML)"]
	case strings.Contains(low, "классификац"):
		return topicCurriculum["Классификация (типы задач ML)"]
	case strings.Contains(low, "регресс"):
		return topicCurriculum["Регрессия (типы задач ML)"]
	case strings.Contains(low, "переобуч") || strings.Contains(low, "overfit"):
		return topicCurriculum["Диагностика переобучения (Overfitting / Underfitting)"]
	case strings.Contains(low, "валидац") || strings.Contains(low, "train") || strings.Contains(low, "test"):
		return topicCurriculum["Схема валидации данных (Train / Val / Test)"]
	case strings.Contains(low, "дисбаланс") || strings.Contains(low, "precision") || strings.Contains(low, "recall"):
		return topicCurriculum["Метрики качества при дисбалансе классов (Precision / Recall / F1)"]
	case strings.Contains(low, "утечк") || strings.Contains(low, "leakage"):
		return topicCurriculum["Утечка целевой переменной (Data Leakage)"]
	case strings.Contains(low, "мнк") || strings.Contains(low, "вырожден"):
		return topicCurriculum["Аналитическое решение МНК и вырожденность матрицы"]
	case strings.Contains(low, "коэффициент") || strings.Contains(low, "интерпретац"):
		return topicCurriculum["Интерпретация коэффициентов линейной модели"]
	case strings.Contains(low, "градиент"):
		return topicCurriculum["Градиентный спуск для линейной регрессии на NumPy"]
	case strings.Contains(low, "регуляриз"):
		return topicCurriculum["L1 и L2 регуляризация весов"]
	default:
		return []string{
			fmt.Sprintf("Теоретические основы и определения темы «%s»", topic),
			"Критерии применения и практические различия со смежными темами",
		}
	}
}

func resolveTopicName(it AnswerFeedbackItem) string {
	if t := strings.TrimSpace(it.Topic); t != "" {
		return t
	}
	q := strings.TrimSpace(it.Question)
	runes := []rune(q)
	if len(runes) > 40 {
		return string(runes[:37]) + "..."
	}
	if len(runes) == 0 {
		return "Тема задания"
	}
	return q
}

func cleanSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".; ")
	s = strings.TrimSpace(s)
	return s
}

func parseEducationalContent(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	var parts []string
	for _, marker := range []string{"Нужно знать:", "Нужно понимать:"} {
		if idx := strings.Index(content, marker); idx != -1 {
			sub := content[idx+len(marker):]
			for _, stop := range []string{"Нужно понимать:", "Нужно уметь:"} {
				if stop != marker {
					if stopIdx := strings.Index(sub, stop); stopIdx != -1 {
						sub = sub[:stopIdx]
					}
				}
			}
			sub = strings.TrimSpace(strings.TrimRight(sub, ".;"))
			if sub != "" {
				sub = strings.ToUpper(string([]rune(sub)[:1])) + string([]rune(sub)[1:])
				parts = append(parts, sub)
			}
		}
	}

	if len(parts) > 0 {
		return parts
	}

	for _, s := range strings.Split(content, ".") {
		s = strings.TrimSpace(s)
		if len(s) > 15 {
			parts = append(parts, s)
			if len(parts) >= 3 {
				break
			}
		}
	}
	return parts
}

func formatGap(topic string, maxScore int, feedback []string, educationalContent string) (string, string) {
	cleaned := make([]string, 0, len(feedback))
	for _, f := range feedback {
		if c := cleanSentence(f); c != "" {
			cleaned = append(cleaned, c)
		}
	}

	subtopics := parseEducationalContent(educationalContent)
	if len(subtopics) == 0 {
		subtopics = resolveTopicSubtopics(topic)
	}

	subtopicsLines := make([]string, 0, len(subtopics))
	for _, s := range subtopics {
		subtopicsLines = append(subtopicsLines, "  — "+s)
	}
	repeatBlock := strings.Join(subtopicsLines, ";\n") + "."

	limit := min(2, len(subtopics))
	recSummary := strings.Join(subtopics[:limit], ", ")

	if len(cleaned) == 0 {
		gap := fmt.Sprintf("Тема «%s» — выявлен пробел (0/%d).\n• В чём ошибка: неверный выбор ответа или отсутствие аргументации.\n• Что повторить по теме:\n%s",
			topic, maxScore, repeatBlock)
		rec := fmt.Sprintf("Закрепить тему «%s»: %s.", topic, recSummary)
		return gap, rec
	}

	verdict := cleaned[0]
	var reason string
	if len(cleaned) >= 2 {
		reason = cleaned[1]
	}

	var mistake string
	lowVerdict := strings.ToLower(verdict)
	if strings.Contains(lowVerdict, ":") {
		parts := strings.SplitN(verdict, ":", 2)
		detail := strings.TrimSpace(parts[1])
		if len(detail) > 0 {
			detail = strings.ToUpper(string([]rune(detail)[:1])) + string([]rune(detail)[1:])
		}
		if reason != "" {
			mistake = fmt.Sprintf("%s. %s", detail, reason)
		} else {
			mistake = detail
		}
	} else if strings.EqualFold(lowVerdict, "ответ неверный") || strings.EqualFold(lowVerdict, "неверно") {
		if reason != "" {
			mistake = reason
		} else {
			mistake = "выбран неверный вариант ответа без достаточного объяснения"
		}
	} else {
		if reason != "" {
			mistake = fmt.Sprintf("%s (%s)", reason, strings.ToLower(verdict))
		} else {
			mistake = verdict
		}
	}
	mistake = strings.TrimRight(mistake, ".")

	gap := fmt.Sprintf("Тема «%s» — выявлен пробел (0/%d).\n• В чём ошибка: %s.\n• Что повторить по теме:\n%s",
		topic, maxScore, mistake, repeatBlock)
	rec := fmt.Sprintf("Закрепить тему «%s»: %s.", topic, recSummary)
	return gap, rec
}

func AnalyzeAnswers(items []AnswerFeedbackItem) SessionFeedbackReport {
	report := SessionFeedbackReport{
		Strengths:       make([]string, 0),
		Gaps:            make([]string, 0),
		Partials:        make([]string, 0),
		Recommendations: make([]string, 0),
		Items:           items,
	}

	for _, it := range items {
		report.Score += it.Score
		report.MaxScore += it.MaxScore

		topic := resolveTopicName(it)

		if it.Score == it.MaxScore && it.MaxScore > 0 {
			report.Strengths = append(report.Strengths, fmt.Sprintf("Тема «%s» — уверенный ответ (%d/%d)", topic, it.Score, it.MaxScore))
		} else if it.Score == 0 {
			gap, rec := formatGap(topic, it.MaxScore, it.Feedback, it.EducationalContent)
			report.Gaps = append(report.Gaps, gap)
			report.Recommendations = append(report.Recommendations, rec)
		} else {
			detail := ""
			if len(it.Feedback) >= 2 && strings.TrimSpace(it.Feedback[1]) != "" {
				detail = fmt.Sprintf("\n• Замечание: %s.", cleanSentence(it.Feedback[1]))
			}
			report.Partials = append(report.Partials, fmt.Sprintf("Тема «%s» — частично освоена (%d/%d).%s", topic, it.Score, it.MaxScore, detail))
			if len(it.Feedback) >= 3 && strings.TrimSpace(it.Feedback[2]) != "" {
				report.Recommendations = append(report.Recommendations, fmt.Sprintf("По теме «%s»: %s.", topic, cleanSentence(it.Feedback[2])))
			} else {
				report.Recommendations = append(report.Recommendations, fmt.Sprintf("Закрепить практическое применение и обоснование в теме «%s».", topic))
			}
		}
	}

	if report.MaxScore > 0 {
		report.ScorePercentage = (report.Score * 100) / report.MaxScore
	}

	return report
}
