# Эталон поведения перед объединением

Дата фиксации: 25.10.2026. Это результаты исходных версий, не отчёт о завершении интеграции.

## Исходные версии и метод

- Обучение: `v0.0.2.5`, `f915c9cc79710d5d7b5c43ac82190f8ca86e9a15`.
- Экзамен и стартовая: `c8f63935a8a80c0d890be70f46479c3bae343129`.
- Временные snapshots для проверки извлечены `git archive` в ignored scratch; это не дополнительные Git worktree и не runtime-зависимость объединённого приложения.
- Исходные тесты запускались независимо от меняющегося рабочего кода.

## Выполненные проверки эталона

| Версия | Проверка | Фактический результат |
| --- | --- | --- |
| Обучение | Backend `go test -race ./... -count=1` с PostgreSQL и S3 | Exit 0; 42 package suites прошли, FAIL нет. |
| Экзамен | Backend `go test -race ./... -count=1` с PostgreSQL и S3 | Exit 0; 26 package suites прошли, FAIL нет. |
| Обучение | Frontend `npm ci`, `npm test -- --run` | Exit 0; 38 test files, 257 tests прошли. |
| Экзамен | Frontend `npm ci`, `npm test -- --run` | Exit 0; 33 test files, 261 tests прошли. |
| Основные экраны | Chromium 151, 1440×1000, ru-RU, Europe/Moscow, light theme | Browser renders сохранены ниже; API подменён фиксированными ответами. |

Backend запускался с одноразовой PostgreSQL 18 и RustFS из того же pinned image, который указан в production Compose. Тестовые контейнеры созданы без подключения существующих project volumes. Model calls в backend tests обслуживались fixture HTTP/gRPC servers, как предусмотрено исходными тестами. Это проверяет adapters, правила, DB и взаимодействие с тестовыми зависимостями, но не доступность настоящих моделей.

Полные baseline-логи текущей рабочей сессии: `.superpowers/sdd/unified-application-plan-2026-10-25/baseline-{learn,exam}-{go,frontend}.log`. Это локальные игнорируемые артефакты; зафиксированные выше команды, SHA и результаты не зависят от их дальнейшего наличия.

## Карта исходников и пользовательских сценариев

Пути в таблице относятся к указанному source ref; их можно читать командой `git show <ref>:<путь>`.

| Сценарий | Источник | Контракты | Где проверять после переноса |
| --- | --- | --- | --- |
| Вход/регистрация/выбор режима | exam ref: `deploy/dual/start/index.html` | Общий auth; один вход вместо двух синхронизируемых аккаунтов | Browser flow `/`, общий auth, screenshots стартовой. |
| Главная обучения/продолжение | learn ref: `services/frontend/src/pages/trainer/ui/learning-home.tsx` | Subjects, learning-state, variant, diagnostic | Learning route tests, reload/intent, cache isolation. |
| Диагностика обучения и голос | learn ref: `.../pages/trainer/ui/diagnostic-trainer.tsx`, `.../features/diagnostic-session` | Variant snapshot, transcription, answer/skip/audio | Перенесённые diagnostic domain/application/integration tests и browser flow. |
| Итог обучения/пробелы | learn ref: `.../pages/trainer/ui/diagnostic-trainer.tsx`; backend `entities/diagnostic/findings.go`, `features/diagnosticfeedback` | Result и stored feedback | Role scores, confirmed gaps, mixed constituents, skip, snapshot regression. |
| План по времени и практика | learn ref: `.../pages/trainer/ui/learning-plan.tsx`, `features/learning-plan`; backend `features/learning` | Plans, messages, open, confirmed answer, retry | Старые learning-plan route/application tests; theory-before-practice browser scenario. |
| Импорт карты обучения | learn ref: `.../pages/competency-map/ui/competency-map-screen.tsx`; backend `features/competency` | CSV/XLSX, subject map | Второй лист, неизвестные колонки, options, предупреждения, переимпорт. |
| Главная и история экзамена | exam ref: `.../pages/trainer/ui/learning-home.tsx` | Subjects, Geek history/start/read | Существующие Geek route tests на новом namespace. |
| Диалог/итог экзамена | exam ref: `.../pages/trainer/ui/geek-{dialog,result}.tsx`; backend `entities/exam` | Geek answer/result/audio/raw | Существующие exam/Geek tests, closing audio, sources, owner/revision. |
| Курсы/материалы/граф экзамена | exam ref: `.../pages/admin-courses`, `.../pages/competency-map` | Materials, metadata, graph build/import | Admin route tests, граф из первого листа, сохранение правил publication. |
| Дополнительные learning API | learn ref: backend `internal/app/app.go` | Training/chat, taskbank/taskgen, full variants/assessment | Прямые contract/integration tests; новых кнопок не добавлять. |

Фактическая UI reachability: старый `LearningHome` переводит из диагностики в `LearningPlanEntry`; самостоятельные training/chat API не подключены к этому UI. Они сохраняются как registered implemented API, чтобы интеграция не урезала backend молча.

## Полный manifest API

[api-manifest.json](integration-parity/api-manifest.json) содержит метод, исходный mux path, целевой API path и режим: 48 learning + 19 exam + 9 общих = 76 операций. Получен из обоих исходных `internal/app/app.go`; catch-all 404 не считается отдельной операцией. Новый сервер должен покрыть manifest, а не только endpoints, попавшие в один happy-path UI test.

## Browser reference screenshots

Ответы API фиксированы: один студент/учитель, один курс, пустая история/материалы и карта без импорта. Поэтому эти снимки пригодны для сравнения главных экранов и административных пустых состояний; они не подтверждают диагностику с реальными данными и не заменяют DB/browser E2E.

- [Стартовая: вход](integration-parity/start-login.png)
- [Стартовая: регистрация](integration-parity/start-register.png)
- [Стартовая: выбор режима](integration-parity/start-choice.png)
- [Обучение: главная](integration-parity/learn-home.png)
- [Обучение: импорт карты](integration-parity/learn-admin.png)
- [Экзамен: главная](integration-parity/exam-home.png)
- [Экзамен: список курсов](integration-parity/exam-courses.png)
- [Экзамен: подготовка курса](integration-parity/exam-admin.png)

[baseline.json](integration-parity/baseline.json) сохраняет browser version, viewport, source refs и извлечённые тексты для этих состояний. Игнорировать случайные ID/даты при последующем сравнении; не обновлять baseline под несогласованный новый дизайн.

## Что ещё необходимо доказать на объединённом приложении

- Сквозной путь: единый вход → обучение → диагностика → итог → план → урок → выбор режима → экзамен → итог → logout.
- Непересечение режимов и student/admin session/cache, в том числе при reload и позднем ответе запроса.
- Восстановление после перезапуска API, переимпорта карты и сбоя модели/S3.
- Реальный микрофон по HTTPS и настоящие STT/TTS/LLM endpoints, если они доступны в окружении.
- Production Docker build и работа единого Compose stack через `dev.sh`/`prod.sh`.

Ни один из этих пунктов не считается выполненным только из-за наличия эталонных screenshots или исходных зелёных unit/integration suites.
