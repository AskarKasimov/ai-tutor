# Introduction to AI: handoff и проверка совместимости

Проверено 16.10.2026 относительно `origin/master` **18620bc** (фикстура и общий prerequisite-граф, PR #89 и #91). Это архив исследования и проверяемая проекция данных, не подключение нового курса к работающему сервису. Документы внутри исходного архива отражают историю проекта и не заменяют действующие требования репозитория. В этой задаче пользователь отдельно требует сохранять версии, provenance и отключённое закрытие предков.

## Что читать

1. [Исходный handoff](AGENT_HANDOFF_COMPETENCY_GRAPH.md).
2. [Канонический пакет](GIC_diagnostic_handoff_v1/README.md), [developer handoff](GIC_diagnostic_handoff_v1/docs/DEVELOPER_HANDOFF.md).
3. [Канонические данные](GIC_diagnostic_handoff_v1/data/course_graph.v1.json) и [provenance](GIC_diagnostic_handoff_v1/data/topic_provenance.v1.json).
4. [Отчёт проверки и сообщение для Феди](REVIEW.md).
5. [Проекция в формат Феди](course-graph.fedia.v1.json) и [воспроизводимый экспорт](export_review.py).

Весь исходный комплект распакован без изменения байтов, включая шесть архивов `history/`, визуализации, схемы и reference engine. Отдельный handoff из Downloads совпадает с архивным по SHA256. Оригинальные PDF не входят в полученный комплект; для сверки формул и показа слайдов нужен `AI_course_cleaned(1).zip`. `.gitattributes` сохраняет оригинальные байты и SHA256 при checkout на Windows/Linux.

## Проверки

Из этой папки, Python **3.11+** (локально использован 3.13.11):

```sh
python -m pip install -r requirements-review.txt
python -m unittest discover -s GIC_diagnostic_handoff_v1/reference -p 'test_*.py' -v
python GIC_diagnostic_handoff_v1/qa/check_artifact.py
python export_review.py --check
python review_observations.py
```

`python export_review.py` пересобирает только проекцию. `--check` проверяет её актуальность без записи. Исходный JSON и существующая десятинодовая фикстура `api/fixtures/course-graph.json` не меняются. Зависимости можно устанавливать в отдельное virtualenv; `.venv/` исключён из Git.

Проекция соответствует форме контракта, но **не готова к выдаче студенту**: английский исходный текст не переведён, вопросы и связи не валидированы, метаданные остаются в каноническом пакете. Автооценивание и закрытие предков этим экспортом не включаются.
