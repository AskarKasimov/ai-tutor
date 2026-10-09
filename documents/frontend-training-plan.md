# Frontend предметов, диагностики и тренировки — план для Luna

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Подключить весь готовый backend к одному real/mock интерфейсу: предмет → диагностика или тренировка → голосовое прохождение.

**Architecture:** Сохраняем React 19, TanStack Query/Router, Radix Themes, i18next и FSD. Новые entity slices `subject` и `training` владеют HTTP-контрактами. Features `subject-selection` и `training` владеют query/mutation и действиями; страница `pages/trainer` компонует выбор, текущую диагностику, preview и тренировку. Bootstrap подключает источники, без общего доменного контекста в shared.

**Tech Stack:** существующие React/TypeScript/Vite/Zod/Radix/Vitest. Новых зависимостей нет.

**Spec:** `documents/multi-subject-plan.md` (F1), `documents/training-backend-plan.md` (F1), текущие `api/openapi.yaml` и `services/frontend/AGENTS.md`. Backend завершён коммитом `3461358`.

## Global Constraints

- Пользователь прямо поручил «распланируй всё для Luna и запусти»: после записи плана начинаем реализацию без дополнительного согласования.
- Главная сначала выбирает предмет, затем действие. Загрузка страницы/каталога/learning-state не вызывает POST варианта, диагностики или тренировки.
- `POST /variants` передаёт обязательный `subject_id`; никакого выбора первого предмета или ML по умолчанию.
- Без завершённой диагностики предмета тренировка недоступна. Начатая новая диагностика не скрывает предыдущую завершённую. Восстановление после обновления вкладки читает существующую сессию, не создаёт новую.
- Перед POST тренировки показываем категории с точными заголовками «Неосвоенные темы» и «Частичные пробелы в знаниях». Отличный результат — «Свободная тренировка» и доступные темы, включая FALSE. Непроверенная исходная оценка null не превращается в 0.
- Тренировка бесконечная: раунд/число ответов/последние баллы и история; никаких «конец курса», completion или фиксированного лимита упражнений.
- Один ответ включает `exercise_id`, аудио и стабильный `Idempotency-Key`. Технический retry отправляет ту же FormData с тем же ключом. Отображённый ответ относится к отправленному упражнению, хотя backend уже вернул следующее.
- Ошибка/смена пользователя/предмета/экрана отменяет запросы и освобождает recorder, player, waveform и object URL. Поздний ответ прошлого auth epoch не обновляет UI или cache нового пользователя.
- Radix, SCSS Modules и i18next; все новые пользовательские строки локализованы. Один h1, доступные названия, клавиатурные button/select.
- FSD: slices не импортируют peers; shared не содержит domain. Публичные API через index.ts. `routeTree.gen.ts` не править вручную.
- Real и mock имеют одинаковые HTTP-контракты. Mock честно обозначает демонстрационные transcript/grade/audio; не имитирует реальную проверку знаний.
- Не менять зависимости, lockfile, auth-модель, legacy-demo или глобальные стили без необходимой причины. Отказ от тихих fallback важнее старых тестов автозапуска: тесты обновляются к новому явному сценарию.

## Review Focus

1. StrictMode, double click и повтор входа: до кнопки нет POST, после кнопки один запуск с устойчивым ключом (Task 3/5).
2. Logout/login другого пользователя и смена предмета во время загрузки/записи: нет чужой сессии, поздних обновлений и неосвобождённых медиа (Task 3/4/5).
3. Переимпорт после preview или при продолжении: stale обновляет preview; существующая тренировка продолжает свой замороженный план (Task 5/6).
4. Принятый ответ, но потерянный HTTP-ответ: retry не создаёт новую попытку; следующий вопрос появляется после просмотра фидбэка (Task 4/5).
5. Пустой каталог, пустой банк, audio missing/pending/failed/cancelled, 401/404/409: осмысленные состояния с retry, текстовый ответ остаётся доступен (Task 3/5).

## Task 1: Typed HTTP API предметов и тренировок

**Files:** создать `services/frontend/src/entities/subject/{index.ts,model/subject.ts,api/subject-api.ts}`, `entities/training/{index.ts,model/training.ts,api/training-api.ts}`; тесты `tests/entities/subject/subject-api.test.ts`, `tests/entities/training/training-api.test.ts`. Существующий diagnostic API пока не менять.

**Interfaces:**

Subject entity экспортирует:
```ts
type Subject = { id: string; name: string; ready: boolean }
type LearningState = {
  subject_id: string; subject_name: string; diagnostic_status: 'not_started'|'active'|'completed'
  diagnostic_completed: boolean; training_available: boolean
  diagnostic_session_id?: string; active_session_id?: string
}
listSubjects(signal: AbortSignal): Promise<Subject[]>
createSubject(name: string, signal: AbortSignal): Promise<Subject>
readLearningState(subjectId: string, signal: AbortSignal): Promise<LearningState>
class SubjectApiError extends Error { status: number; code: string }
```

Training entity экспортирует wire types из `entities/training/model.go` backend:
`TrainingTarget` (kind/label/competency/outcome/nullable original_score/original_max_score/last_score),
`TrainingExercise` (exercise_id/outcome/question/options/voice_instruction),
`TrainingAttempt` (sequence/exercise_id/round/target_index/transcription_id/text/score/max_score/verdict/criterion_results/feedback/created_at),
`TrainingProgress` (session_id/diagnostic_session_id/subject_id/subject_name/mode/status=active/round/answer_count/targets/current/optional answer),
`TrainingPreview` (diagnostic_session_id/subject_id/name/mode/plan_revision/diagnostic_score/maximum_score/status=ready|no_practice_tasks/confirmed_gaps/partial_competencies/topics),
`TrainingHistory` (items/targets/optional next_cursor).

```ts
type TrainingSubmission = { sessionId: string; exerciseId: string; key: string; body: FormData }
readTrainingPreview(diagnosticId: string, signal: AbortSignal): Promise<TrainingPreview>
startTraining(diagnosticId: string, key: string, planRevision: number | undefined, signal: AbortSignal): Promise<TrainingProgress>
findTrainingForDiagnostic(diagnosticId: string, signal: AbortSignal): Promise<TrainingProgress>
readTrainingSession(sessionId: string, signal: AbortSignal): Promise<TrainingProgress>
createTrainingSubmission(sessionId: string, exerciseId: string, blob: Blob): TrainingSubmission
submitTraining(input: TrainingSubmission, signal: AbortSignal): Promise<TrainingProgress>
readTrainingHistory(sessionId: string, cursor: string | undefined, limit: number, signal: AbortSignal): Promise<TrainingHistory>
readTrainingAudio(sessionId: string, exerciseId: string, signal: AbortSignal): Promise<TrainingAudioMetadata>
fetchTrainingAudioFile(audioUrl: string, signal: AbortSignal): Promise<Blob>
class TrainingApiError extends Error { status: number; code: string }
```

- [ ] Написать тесты реального fetch boundary: encoded IDs, cookie auth, POST JSON/plan_revision, multipart ровно audio/exercise_id, повтор submission использует тот же ключ; malformed success, wrong session/exercise ID, nullable исходный балл, 409 code.
- [ ] `npm test -- --run tests/entities/subject/subject-api.test.ts tests/entities/training/training-api.test.ts` → сначала RED.
- [ ] Реализовать Zod schemas и функции, по образцу diagnostic API. 30s GET, 120s start, 240s answer; `apiFetch`, AbortSignal.any, `fetchStoredAudio`. Audio URL только `/task-audio/<id>/file`. Не выдавать private snapshot. Отправлять `{}` для focused POST и `{plan_revision}` для free. Проверять returned IDs, score/max_score=2, режим/status/counters, verdict/feedback.
- [ ] Те же тесты → GREEN; `npm run typecheck`, `npm test -- --run`; форматировать только затронутые файлы; commit `feat(frontend): add subject and training API contracts`.

## Task 2: Mock HTTP для предметов и бесконечной тренировки

**Files:** `src/bootstrap/mock-api.ts`, `mock-diagnostic-api.ts`; новые `mock-subject-api.ts`, `mock-training-api.ts` при необходимости; `tests/bootstrap/mock-learning.test.ts`. Не менять UI/hook lifecycle.

**Consumes:** Task 1 wire types. **Produces:** те же /subjects, learning-state и training маршруты в `mockApiFetch`; subject id `subject:intro-to-ml`, название «Введение в ML». Разделить API state по текущему userId, передаваемому из mock-api; logout/login очищает mutable mock state.

- [ ] RED: через реальные entity API + configured mock transport проверить до диагностики gate, явное создание варианта с subject_id, completed learning-state, preview без записи, free_practice, один start на диагностику, новый exercise_id/round после ответа, повтор ключа и конфликт иного аудио, курсор истории. Фикстуры focused preview с обоими типами проверяются отдельным API test; backend classification не дублировать в сложный mock rule engine.
- [ ] `npm test -- --run tests/bootstrap/mock-learning.test.ts` → RED.
- [ ] Дополнить existing mock diagnostic state: subject ID у варианта/сессии, start/answer ключи, данные для learning-state. `mock-training-api` хранит только mock сессии/accepted attempts, отдаёт контракты Task 1, 404/409 при неизвестной/незавершённой диагностике. Формировать демонстрационную оценку и transcript, new exercise каждый round; audio использует существующий createDemoAudio. Админские subject create/map routes можно поддержать, но не менять mock auth role student.
- [ ] Tests → GREEN; typecheck/full tests; commit `feat(frontend): mock subject learning and training APIs`.

## Task 3: Главная с выбором предмета и явной диагностикой

**Files:** новые `src/features/subject-selection/{index.ts,model/dependencies-context.tsx,model/query-keys.ts,model/use-subjects.ts}`, `src/pages/trainer/ui/learning-home.tsx`, `learning-home.module.scss`; изменить `trainer-screen.tsx`, `diagnostic-trainer.tsx`, diagnostic identity/storage/start/use-session, `entities/diagnostic-session/api/diagnostic-api.ts`, bootstrap dependencies/providers/mock-storage, i18n. Тесты `tests/routes/subjects.test.tsx`, existing diagnostic/mock routes и tests/features/diagnostic-session.

**Consumes:** Task1 subjects, Task2 mock. **Produces:** главный экран; props `DiagnosticTrainer({subjectId, subjectName, onBack, initialSessionId?})`; subject-selection provider `{subjects:{listSubjects,createSubject,readLearningState}}`, экспорт hooks `useSubjectsQuery(userId)`, `useLearningStateQuery(userId,subjectId)`, `useCreateSubjectMutation(userId)`. Keys включают userId/subjectId. Bootstrap AppDependencies добавляет `subjects`, заполняет и providers и test fixtures.

Diagnostic signatures одновременно обновляются у всех callers/ports/tests:
```ts
createVariant(subjectId: string, key: string, signal: AbortSignal): Promise<{id:string}>
createDiagnosticIdentity(subjectId: string): DiagnosticSessionIdentity
// DiagnosticSessionIdentity добавляет обязательный subjectId
// storage load/save ключ: apiBase + userId + subjectId; забытые старые записи без subjectId игнорируются
useDiagnosticSession(userId: string, subjectId: string, initialSessionId?: string)
```

- [ ] RED: root после auth показывает предметы и не вызывает POST; выбор предмета грузит только learning-state. Кнопка диагностики вызывает createVariant с выбранным ID; StrictMode/doubleclick один start. Без completed тренировка заблокирована и есть понятное «Сначала пройдите диагностику». Ошибка/пустой каталог, неизвестный subject, retry. Logout/new user и старые sessionStorage записи не запускают чужую сессию.
- [ ] Запустить `npm test -- --run tests/routes/subjects.test.tsx` → RED.
- [ ] Главная — Radix select/cards + два режима и состояние learning-state. Не монтировать DiagnosticTrainer до explicit start/continue. Кнопка «Продолжить диагностику» берёт active_session_id и выполняет GET; «Новая диагностика» на итоговом экране возвращает к выбору, не вызывает POST. Subject name приходит из каталога/снимка, hardcoded session.course заменяется props.
- [ ] Устойчивые ключи создания сохранять перед первым POST; retry загрузки использует их. Для refresh сохранять page intent в sessionStorage с apiBase/userId/subjectId/sessionId; восстанавливать только existing ID и выбранный предмет, никогда автоматически создавать сессию. Возврат к выбору очищает page intent, сохраняет diagnostic identity для осмысленного continue. Auth epoch guards и отмена запросов как existing hooks.
- [ ] Временно кнопка доступной тренировки может открывать entry placeholder с локализованной loading подписью; Task5 его заменяет. Не выдавать демонстрационную тренировку за real.
- [ ] Обновить старые тесты: перед первым вопросом явно выбрать subject и нажать start; сохранить проверки голоса/result/privacy. tests → GREEN; typecheck/full tests; commit `feat(frontend): select subjects and explicitly start diagnostics`.

## Task 4: Training dependencies, queries и голосовой lifecycle

**Files:** новые `src/features/training/{index.ts,model/dependencies-context.tsx,model/query-keys.ts,model/use-training-session.ts,model/use-training-voice.ts}`; bootstrap deps/providers; tests `tests/features/training/use-training-session.test.tsx`, `use-training-voice.test.tsx`.

**Consumes:** Task1 Training API и shared/lib browser audio; Task3 subject hooks. **Produces:** feature provider `{training: TrainingApi}` (тип назван `TrainingDependencies`), exported hooks `useTrainingEntryQuery(userId,diagnosticId)`, `useStartTrainingMutation(userId,diagnosticId)`, `useTrainingSession(userId,sessionId)`, `useTrainingHistoryQuery(userId,sessionId)`, `useTrainingVoice(userId,progress,submit,onError)`.

Entry query сначала GET existing training: 404 означает отсутствие и только тогда GET preview; найденная сессия показывает свои frozen targets после переимпорта. Возвращает discriminated union `{kind:'existing';progress}` | `{kind:'preview';preview}`.

- [ ] RED: accepted response updates correct cache; start key survives failed HTTP retry; stale returns user to new preview; entry existing does not POST or use current-map topics; wrong auth epoch ignored. Voice test: permission→recording→processing→result; retry keeps submission; feedback stays bound to captured exercise until Next; changing task/unmount cleans resources, late result ignored.
- [ ] Run focused tests → RED.
- [ ] Query/mutation implementations use injected API, user/session keys and existing captureSession/registerSessionRequest/isCurrentSession patterns. History useInfiniteQuery with next_cursor. Register every write with auth token; 401 replaces auth only for current epoch.
- [ ] Voice hook uses shared `startRecording`, `createAudioUrl`, `playQuestion` and waveform in UI; не импортировать feature diagnostic-session. Media refs and generation counter отменяют поздние callbacks. Current audio statuses render hints, no TTS retry/regenerate endpoint invented. Source metadata ready fetch WAV через existing shared fetchStoredAudio. Unavailable audio не блокирует запись. Browser permission/empty audio/limit/error has retry path. Pending answer survives technical retry within screen with same FormData/key.
- [ ] Commit after focused/typecheck/full tests: `feat(frontend): manage persistent training and voice answers`.

## Task 5: Preview, бесконечная тренировка и история на главной

**Files:** новые `src/pages/trainer/ui/training-preview.tsx`, `training-session.tsx`, `training-history.tsx`, `training.module.scss`; обновить learning-home, i18n; tests `tests/routes/training.test.tsx`, `mock-diagnostic.test.tsx`.

**Consumes:** Task3 home, Task4 hooks. **Produces:** полный root сценарий в real и mock.

- [ ] RED: absent/incomplete diagnostic gate; focused preview с обеими точными категориями и исходными баллами; free preview с null score не показывает 0; preview GET не вызывает start. Existing session показывает frozen targets + Continue. Explicit button POST revision/key. 409 stale refetch topics and requires another click. no_practice_tasks disables start; network error keeps user choice.
- [ ] Focused tests → RED.
- [ ] Preview shows subject, topics, categories and explicit «Начать тренировку»/«Продолжить тренировку». Home passes latest completed diagnostic_session_id. Return from diagnostic invalidates learning-state before gate. UI does not infer mastery or calculate gaps itself.
- [ ] Session shows subject, current outcome/question/options/voice_instruction, round/count, recorder/waveform/listen, 0/1/2 feedback/transcript, explicit Next, goal latest score, paginated history with «Показать ещё». Нет finite progress percentage or completion. Cancel recording before Back/switch, preserve session intent for refresh; reload GET current session and history. Back returns subject/action selector; session remains backend active.
- [ ] Error actions: 401 auth; 404 clear current view and offer subject selector; 409 wrong exercise refetch current; in-progress/retry mismatch preserve original submission with explanatory text. Respect voice hook generation to prevent another goal receiving old recording/result. Audio play requires click; missing/pending/failed/cancelled shows status with text fallback.
- [ ] tests → GREEN; run full frontend gates; commit `feat(frontend): show training preview progression and history`.

## Task 6: Админские предметы и предметный импорт

**Files:** `src/entities/competency-map/api/competency-map-api.ts`, `src/features/import-competency-map/model/{dependencies-context,query-keys,use-competency-map}.ts(x)`, `src/pages/competency-map/ui/competency-map-screen.tsx`, i18n, bootstrap wiring; `tests/routes/admin.test.tsx`, API/hook tests.

**Consumes:** Task1 subjects, Task3 subject hooks. **Produces:** admin sees all subjects including ready=false, creates subject, selects map/import target.
```ts
readCompetencyMap(subjectId: string, signal: AbortSignal): Promise<CompetencyMapSummary>
importCompetencyMap(subjectId: string, file: File, signal: AbortSignal): Promise<CompetencyMapImport>
useCompetencyMapQuery(userId: string, subjectId: string)
useImportCompetencyMapMutation(userId: string, subjectId: string)
```

- [ ] RED: explicit subject select, create validates name/errors, empty new map state, imported ID in encoded URL, A summary never replaces B after switch, import request tied to captured subject. Admin auth/401/403 tests preserved.
- [ ] Focused tests → RED.
- [ ] Subject create/list selection on existing admin screen, preserve CSV/XLSX picker and warnings. All API paths `/subjects/{id}/competency-map` and `/admin/subjects/{id}/competency-map/import`. Keys include subject ID. Successful/uncertain import invalidates selected map, subject readiness and learning-state; late success must not display under another subject.
- [ ] tests → GREEN; full gates; commit `feat(frontend): administer subject maps and imports`.

## Task 7: Контрактная интеграция, визуальная проверка и документация

**Files:** frontend tests/routes, docs and README; if removing legacy routes: backend `app/app.go`, competency HTTP annotations, relevant API tests and `api/openapi.yaml`.

**Consumes:** Tasks1–6; **Produces:** complete F1, updated plans and documented UI real/mock behavior.

- [ ] Verify no new frontend caller uses global `/competency-map` or empty-body `POST /variants`; no real path reaches legacy-demo.
- [ ] Remove obsolete global competency-map HTTP aliases as agreed in multi-subject spec, migrate affected backend test URLs explicitly to `subject:intro-to-ml`, regenerate OpenAPI. Preserve schema/data/history. This is scoped route cleanup, not migration/table removal. Run affected backend tests with disposable PostgreSQL, then go test ./..., go vet, build.
- [ ] Run frontend gates exactly: `npm run format:check`, `npm run lint`, `npm run typecheck`, `npm test -- --run`, `npm run build`. Resolve errors, no disables.
- [ ] Preview UI through available browser tools on local Vite: keyboard subject selection, mode buttons, preview, microphone denied/text/audio states, narrow viewport, light/dark styles. If browser tool access unavailable, record limitation and rely on route tests/build; do not claim visual verification.
- [ ] Update root/frontend README, `documents/voice-trainer-scenarios.md`, mark F1 complete in both backend plans. Document data durability difference: real PostgreSQL; mock browser demo resets on new login. LLM analogous exercises remain future provider; don't promise fresh question content.
- [ ] Commit `docs(frontend): document subject diagnostic and training flows`; final fresh review across frontend range and scoped contract changes. Fix correctness findings and rerun covering tests/full gates.

## Execution Notes

Sequential Luna tasks 1→2→3→4→5→6→7; do not parallelize implementers sharing bootstrap/i18n/mock files. Each worker reads its extracted brief, AGENTS.md, frontend AGENTS and named files/tests, not all prior conversation. Escalate missing context to controller; do not ask the user again for already specified behavior. Write report in the plan workspace, include changed files/tests/commit/concerns; never commit scratch reports. Review each task for spec and quality before the next.
