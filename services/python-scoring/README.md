# AI Tutor scoring module

A standalone Python module that accepts one student answer as JSON and returns a mark plus three Russian feedback lines. It has no HTTP server and no dependency on the Go backend.

## Python 3.12 setup

Run these commands from this directory:

If `.venv` was created with another Python version, remove it first. A virtual environment cannot be converted in place.

```bash
cd /Users/ivan/IdeaProjects/ai-tutor/services/python-scoring
deactivate 2>/dev/null || true
rm -rf .venv
python3.12 -m venv .venv
source .venv/bin/activate
python --version
# Python 3.12.x
python -m pip install --upgrade pip
python -m pip install -e .
python -m unittest discover -s tests -v
```

Confirm the active interpreter with:

```bash
python --version
# Python 3.12.x
```

## Input contract

```json
{
  "Question": "Чем классификация отличается от регрессии?",
  "Answer": "Классификация выбирает класс, а регрессия предсказывает число.",
  "Дисциплина": "Прикладная статистика",
  "Образовательные результаты": [
    "Объясняет разницу между классификацией и регрессией."
  ]
}
```

All four fields are required. `Образовательные результаты` can be either one non-empty string or an array of up to 20 non-empty strings. Unknown fields are rejected.

## Calling a real model

Set the private model endpoint and model name; never place provider credentials in frontend variables.

```bash
export ASSESSMENT_BASE_URL='http://your-llm-host/v1'
export ASSESSMENT_MODEL='gpt-oss-120b'
```

```python
from ai_tutor_scoring import LLMSettings, OpenAICompatibleGrader, score

result = score(payload, OpenAICompatibleGrader(LLMSettings.from_env()))
print(result)
```

The result always has this form:

```json
{
  "mark": 0,
  "feedback": ["Вердикт.", "Причина.", "Что закрепить."]
}
```

`mark` is `0`, `1`, or `2`. `feedback` always contains exactly three non-empty, one-line Russian strings.
