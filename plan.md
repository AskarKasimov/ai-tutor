# План: общий итоговый экран диагностики и экзамена

Статус: реализовано в ветке `feat/unified-result-report`. Решения записаны в [README](README.md), раздел «Решения 01.11.2026». Макет: [вариант A](https://claude.ai/artifact/8CHmvJHnwYFFbKvzow9Sni).

## Цель

Итоговый экран оценки с фидбэком одинаковый в диагностике обучения (`/learn`) и симуляции экзамена (`/exam`). Ход диагностики и экзамена, диалог и оценивание ответов не меняются.

## Принятые решения

- Разделы экрана: кольцо с общей оценкой, статусы, общий вывод, «Что получилось», «Что доработать», темы в баллах, граф тем.
- Общая оценка в обеих частях — буква. Рядом сумма баллов: «B, 7 из 9 баллов». Кольцо заполняется по реальному проценту.
- Перевод одинаковый: от 90% — A, от 70% — B, от 60% — C, ниже 60% — D. Других условий для A нет.
- Темы и задания показываются только в баллах, без букв.
- «Что доработать» показывается и при A, если есть неполные ответы.
- Общий вывод пишет модель через LLM gateway при открытии итога. Структура текста одинаковая: общая картина, что получилось, что подтянуть и главный следующий шаг. Обращение на «вы».
- Баллы и причины студент видит только на итоговом экране.
- В сумму баллов экзамена входят все спрошенные темы, как и в диагностике. Темы со статусами «недостаточно данных», «засчитано» и не спрошенные не входят. Шкалы заданий остаются разными.

## Этап 1. Общая оценка и новые поля итога

| Где | Что меняем |
| --- | --- |
| `services/backend/internal/entities/exam/grading.go`, `exam.go` | `CourseGrade` считает процент по сумме баллов всех спрошенных тем и переводит его по порогам. Новый подсчёт баллов темы: сумма числовой рубрики или проверенных К1–К4. Оценки тем, статусы и обход не меняются. |
| `services/backend/internal/entities/exam/exam_test.go` | Случаи подсчёта, границы порогов, состав суммы. |
| `services/backend/internal/features/diagnostic/transport/http/dto.go` | Перевод процента в букву через общий `exam.CourseGrade`: сущности не могут импортировать друг друга. |
| `services/backend/internal/features/diagnostic/transport/http/dto.go` | В итог диагностики добавить `grade` и `score_percent`. |
| `services/backend/internal/features/geek/transport/http/handler.go`, `internal/entities/exam/session.go` | В итог экзамена добавить по каждой теме баллы, максимум, причины рубрики или К1–К4 и реплику Geek при закрытии темы. |
| `api/openapi.yaml` | Перегенерировать, обновить контрактные тесты. |

Оценка считается при чтении, поэтому завершённые экзамены получат новую букву при открытии.

## Этап 2. Общий вывод моделью

| Где | Что меняем |
| --- | --- |
| `services/backend/internal/entities/resultsummary` (новый) | Общий формат отчёта и промпт с единой структурой текста. |
| `services/backend/internal/features/diagnosticfeedback` | Перейти на общий промпт вместо промпта про «курс по машинному обучению». Поднять `analysisVersion`: сохранённые выводы пересоздадутся при открытии. |
| `services/backend/internal/features/geek` | Новая ручка `GET /exam/diagnostic-sessions/{id}/feedback`: собирает отчёт, сохраняет текст. Сбой модели — 503 с возможностью повтора. |
| `services/backend/internal/shared/postgres/migrations/00015_*`, `queries/` | Таблица выводов экзамена, SQLC. Номер миграции уточнить при мерже. |
| `api/openapi.yaml`, `services/backend/README.md` | Новая ручка со статусом `implemented`, раздел «БД и импорт». |

Модель вызывается при открытии итога, а не в диалоге. Оценка и темы видны сразу, абзац загружается отдельно и при сбое показывает кнопку повтора.

## Этап 3. Виджет и экран экзамена

| Где | Что меняем |
| --- | --- |
| `services/frontend/src/widgets/result-report/` (новый) | Общий экран варианта A и его стили. |
| `src/pages/trainer/ui/result-graph.tsx`, `src/pages/trainer/lib/result-graph.ts`, `tests/pages/trainer/lib/result-graph.test.ts` | Перенести граф в виджет; в карточках баллы вместо букв. |
| `src/pages/trainer/lib/to-result-report.ts` (новый) | Перевод итога экзамена в формат экрана. |
| `src/pages/trainer/ui/geek-result.tsx` | Использовать общий экран. Материалы и кнопки остаются. |
| `src/entities/diagnostic-session`, `src/features/diagnostic-session` | Новые поля и запрос общего вывода. |
| `src/shared/i18n/i18n.ts`, `geek-result.module.scss` | Общие строки, удаление лишних стилей. |

Статусы на экране: «Полный ответ», «Частично», «Пробел», «Засчитано», при необходимости «Нет данных». Ответ с помощью — пометка в строке темы.

## Этап 4. Экран диагностики

| Где | Что меняем |
| --- | --- |
| `src/pages/learning-trainer/model/to-result-report.ts` (новый) | Перевод итога диагностики в формат экрана. Граф: основное задание опирается на два базовых своей составляющей. Базовые задания, которые не спросили после полного основного ответа, — «Засчитано». |
| `src/pages/learning-trainer/ui/diagnostic-trainer.tsx` | Использовать общий экран в `DiagnosticSummary`. Кнопка тренировки и разбор ответов с озвучкой остаются. |
| `src/entities/learning-diagnostic-session` | Новые поля итога. |
| `src/shared/i18n/learning-translations.ts`, `diagnostic.module.scss` | Строки и удаление лишних стилей. |
| `tests/` | Тесты виджета и обоих переводов данных. |

## Этап 5. Документация и проверки

Обновить README (описание оценки курса и «Текущая реализация»), README сервисов, `api/README.md`, `checklist.html`.

Проверки backend из `services/backend`:

```bash
go test -race ./... -count=1   # с TEST_DATABASE_URL
go vet ./...
go build -o bin/api ./cmd/api
bash scripts/generate-openapi.sh
```

Проверки frontend из `services/frontend`:

```bash
npm run format:check
npm run lint
npm run typecheck
npm test -- --run
npm run build
```

Отдельно подтвердить фактическим запросом генерацию вывода через gateway и применение миграции.

## Что не трогаем

- Диалог, голосовые реплики, ход экзамена и диагностики.
- Оценщики ответов и их промпты: `features/assessment/infrastructure/modelapi/client.go`, `features/geek/infrastructure/llm/prompts.go`, `rubric.go`.
- Оценки тем, статусы, обход, план обучения и тренировку.
- Импорт карт и выбор заданий.
