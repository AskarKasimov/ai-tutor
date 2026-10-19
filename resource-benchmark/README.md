# Замер ресурсов ai-tutor / Geek — промежуточный отчёт

Статус на 19.10.2026: **замер не закончен.** Выполнена примерно половина
сценариев; вывод для серверной заявки ещё не сформирован. Все цифры ниже
измерены, но по CPU OCR есть один прогон из трёх. Список пробелов — в конце.

## Окружение

Подробно — [raw/environment.json](raw/environment.json).

| | |
| --- | --- |
| Код | `master` `e5890ed`; незакоммиченное: только CRLF в `routeTree.gen.ts` (на результат не влияет) и эта папка |
| CPU | AMD Ryzen 7 260, 8 физических ядер / 16 потоков |
| RAM | 15,3 ГиБ (2 × 8 ГиБ DDR5-5600) |
| GPU | NVIDIA RTX 5060 Laptop, 8 ГиБ VRAM, драйвер 610.71, CUDA cc 12.0 |
| Диск | NVMe SSD 512 ГБ, свободно ~25 ГБ |
| ОС | Windows 11 Pro 26200, WSL 2.6.3 (ядро 6.6.87), Docker 29.2.1, Compose 5.0.2 |
| Лимиты | WSL: `memory=10GB`, `swap=8GB` → Docker VM видит 16 CPU и 9,7 ГиБ; контейнер course-graph — 8 ГиБ (по умолчанию production), у остальных лимитов нет |

Локально работают только модели course-graph: Docling layout-heron, docling-models,
CodeFormulaV2 (VLM формул и кода), RapidOCR, эмбеддинги multilingual-e5-small.
**Удалённо** (университетская сеть 10.100.x, ресурсы не входят в локальные цифры):
LLM через gateway (пулы low/medium/high — gemma, qwen-3-6, gpt-oss-120b,
qwen3-coder-next), STT и TTS.

## Как измерялось

- **Production-вариант**, а не dev: те же `services/*/docker-compose.yaml` и
  порядок, что у `prod.sh` (собранный frontend на nginx, бинарник backend, без
  Air/Vite/watchers). Стенд изолирован от реальных данных: проекты `aitbench-*`,
  сеть `aitbench_default`, volumes `aitbench_*` (`compose/`), тестовые секреты
  в `bench.env` (не коммитится, создаётся `scripts/make-env.sh`). S3 — локальный
  RustFS. Dev-стек на время замера остановлен (`stop`, данные целы).
- **Метрики раз в секунду** (`scripts/monitor.sh`):
  - cgroup v2 каждого контейнера: working set, anon, `memory.peak` (ловит
    пики короче секунды), swap, CPU, throttling, oom_kill, рестарты;
  - Docker VM целиком: `/proc/meminfo`, `/proc/stat`;
  - Windows: свободная память, рабочий набор `vmmemWSL`, загрузка CPU;
  - GPU: nvidia-smi.

  CPU — в логических ядрах: 1,0 — один поток занят полностью, всего их 16.
  Уровни хоста, VM и контейнеров пересекаются, поэтому не складываются.
- Тестовые пользователи `bench-*@example.edu` и курсы `BENCH …` создаются через
  публичный API (`scripts/diag.py`, `scripts/pipeline.py`).

## Результаты

### Сборка и запуск ([raw/00-build](raw/00-build), [raw/01-startup](raw/01-startup))

| Шаг | Время |
| --- | ---: |
| Сборка образа course-graph GPU (cu128) с нуля | 23,2 мин (образ 12,6 ГБ) |
| Сборка образа course-graph CPU | 5,3 мин (образ 2,65 ГБ) |
| Сборка gateway / backend / frontend | 51 / 53 / 36 с |
| `up --wait` всех сервисов в порядке prod.sh | 95 с |
| Скачивание и прогрев моделей OCR (1,17 ГБ) после старта | ≈ 3 мин |

### Зависимости — настоящими запросами ([raw/02-deps](raw/02-deps))

Backend health, миграции PostgreSQL (9), запись в БД, LLM low/medium/high
(≈ 0,35 с на короткий ответ), TTS (15 с на одну фразу), STT (0,8 с, текст
распознан дословно), готовность моделей OCR — всё **OK**. S3 проверен записью
и чтением аудио реплик в диагностике.

### А. Простой 5 минут ([raw/04-idle](raw/04-idle))

| Контейнер | RAM обычно | Пик (cgroup) | CPU в среднем |
| --- | ---: | ---: | ---: |
| course-graph (модели загружены) | 1 284 МиБ | 2 566 МиБ¹ | 0,007 |
| redpanda (очередь gateway) | 757 МиБ | 813 МиБ | 0,014 |
| s3 (RustFS) | 178 МиБ | 243 МиБ | 0,005 |
| postgresql | 67 МиБ | 123 МиБ | 0,008 |
| caddy / frontend / backend / gateway | 24 / 14 / 13 / 8 МиБ | — | < 0,01 |
| **Весь стек** | **2 344 МиБ** | 2 356 МиБ (макс. за окно) | 0,05 |

¹ Пик с момента старта, включая прогрев моделей. VRAM в простое — 1,76 ГиБ
(course-graph держит модели на GPU). Docker VM — 3,1 ГиБ использовано.

### Б. Диагностика ([raw/05-diag-text](raw/05-diag-text), [raw/06-diag-voice](raw/06-diag-voice))

Полная диагностика одного тестового студента (8 ответов: сильный, слабый,
«не знаю»), курс на графе по умолчанию.

| | Текст | Голос |
| --- | ---: | ---: |
| Длительность сессии | 10 с | 149 с |
| Оценка ответа (LLM, удалённо), медиана / макс. | 1,1 / 3,9 с | 1,0 / 1,8 с |
| Озвучка реплики (TTS, удалённо), медиана / макс. | — | 18,2 / 21,7 с |
| Распознавание ответа (STT, удалённо), медиана / макс. | — | 0,39 / 0,73 с |
| Backend, пик RAM / CPU | 80 МиБ / 0,3 ядра | 177 МиБ / 1,0 ядро |
| Весь стек, макс. RAM | 2,42 ГиБ | 2,52 ГиБ |

Открытие приложения: index + 11 ассетов, по 30–50 мс. Локальные ресурсы
диагностики почти не растут; время определяют удалённые модели, прежде всего TTS.

### В. OCR ([raw/07-ocr-gpu](raw/07-ocr-gpu), [raw/08-ocr-cpu](raw/08-ocr-cpu))

Каждый прогон — новый контейнер production-образа с лимитом 8 ГиБ и 4 OCR-потоками.
Кеш результата — временная папка на каждый документ: служебный кеш OCR не
используется и не удаляется. Сервис course-graph на время замера остановлен.
Настройки: `ocr-v9-content-cache`, `formula-audit-v5`, обогащение кода и формул
включено, текст с картинок включён, без полностраничного OCR. Другие кеши, которые
влияют на замер: веса HF в volume моделей (уже скачаны), модели RapidOCR,
которые каждый новый контейнер качает заново, page cache ОС.

| PDF (SHA-256, начало) | Стр. | Тип | GPU, 3 прогона: среднее (мин–макс) | CPU, 1 прогон |
| --- | ---: | --- | ---: | ---: |
| Week 3 Lecture.pdf (`08c27485`) | 27 | обычная лекция | 12,5 с (8,1–20,0) | 60,7 с |
| Tutorial 2.pdf (`a5356b8d`) | 50 | формулы | 91,1 с (65,3–117,1) | 418,8 с |
| Tutorial 4. Prolog.pdf (`db2ac5f1`) | 28 | код Prolog, 8 стр. без текста | 11,8 с (11,2–12,1) | 114,9 с |
| Week 1 Tutorial_2025.pdf (`ad0dba6b`) | 35 | сканы и крупные изображения, 19,8 МБ | 23,0 с (20,2–24,5) | 102,2 с |
| **Всего 140 стр.** | | | **138 с (109–169), ≈ 1,0 с/стр.** | **697 с, ≈ 5,0 с/стр.** |

| | GPU | CPU |
| --- | ---: | ---: |
| Инициализация моделей в новом процессе | 61 с (53–73) | 77 с |
| Первая конвертация (1 стр., прогрев) | 15–81 с | 47 с |
| Повторный запрос с готовым кешем | 1–20 мс | 2–18 мс |
| Пик RAM контейнера (cgroup) | 3,8–4,0 ГиБ | 5,8 ГиБ |
| VRAM: torch reserved / вся GPU | 5,6 / 5,9 ГиБ | — |
| CPU: среднее / пик | 1,1 / 5,4 ядра | 3,8 / 12,5 ядра² |

² Ограничение в 4 OCR-потока соблюдается не всеми библиотеками.

Markdown CPU и GPU побайтно совпал на лекции и документе с формулами. В Prolog
расходятся 4 строки, в сканах — 14. Между тремя прогонами на GPU результат
стабилен. `content_complete=false` у документа с формулами и у Prolog: OCR сам
помечает фрагменты, которые не смог подтвердить.

## Не измерено (пробелы на текущий момент)

- CPU OCR: 2 из 3 прогонов не выполнены. Второй прогон остановлен, когда у Windows
  кончилась свободная память (минимум 121 МиБ при 6,3 ГиБ у WSL VM) и Claude Code
  снял фоновые задачи.
- Г. Конвейер Михаила (загрузка → OCR → перевод → очистка/темы → связи → граф):
  на небольшом наборе и на полном курсе (28 PDF) — не запускался.
- Д. Совместная нагрузка (1/3/5 диагностик во время обработки курса) — не запускалась.
- Диск «после обработки курса» — не снят; есть только «до» ([raw/03-disk](raw/03-disk)).
- Минимальная конфигурация не проверялась (например, с урезанными лимитами).
- Вывод для заявки по четырём вопросам — не сформирован.

## Воспроизведение

```bash
cd resource-benchmark
COURSE_GRAPH_DEVICE=gpu scripts/make-env.sh          # bench.env с тестовыми секретами
docker compose stop                                  # из корня: dev-стек делит порты
scripts/stack.sh build raw/00-build
scripts/monitor.sh start "$PWD/raw/01-startup"; scripts/stack.sh up raw/01-startup
python scripts/deps.py --out raw/02-deps/deps.jsonl
scripts/disk.sh raw/03-disk/before
scripts/monitor.sh start "$PWD/raw/05-diag-text"
python scripts/diag.py --open-app --out raw/05-diag-text/requests.jsonl
python scripts/diag.py --sessions 1 --mode text --out raw/05-diag-text/requests.jsonl
python scripts/diag.py --sessions 1 --mode voice --out raw/06-diag-voice/requests.jsonl
scripts/stack.sh compose course-graph stop           # OCR без сервиса
scripts/ocr.sh gpu 3 "$PWD/raw/07-ocr-gpu" ../documents/materials/{"Week 3 Lecture","Tutorial 2","Tutorial 4. Prolog","Week 1 Tutorial_2025"}.pdf
scripts/monitor.sh stop <run-dir>
python scripts/analyze.py <run-dir> --from <ts> --to <ts> > summary.json; python scripts/show.py summary.json
python scripts/latency.py <run-dir>/requests.jsonl
python scripts/ocr_summary.py "raw/07-ocr-gpu/gpu-run*.json"
scripts/stack.sh stop; docker compose start         # вернуть dev-стек
```

Окна сценариев берутся из `events.csv` каждой папки `raw/`.
