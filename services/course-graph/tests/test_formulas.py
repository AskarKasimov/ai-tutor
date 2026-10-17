"""Tests for LLM-based recovery of undecoded OCR formulas."""
import asyncio
import json
import pytest

from course_graph.ocr.formulas import recover_formulas


def test_markdown_without_undecoded_formulas_is_unchanged():
    md = "# Заголовок\n\nОбычный текст с формулой $x^2 + y^2 = 1$."

    async def dummy_complete(system, text):
        raise AssertionError("LLM must not be called when no undecoded formulas exist")

    result = asyncio.run(recover_formulas(md, dummy_complete))
    assert result == md


def test_recovers_undecoded_formula_from_slide_context():
    slide = (
        "## Слайд 14\n\n"
        "### Среднеквадратичная ошибка (MSE)\n\n"
        "Формула ошибки:\n\n"
        "<!-- formula-not-decoded -->\n\n"
        "где y_i — правильный ответ, \\hat{y}_i — прогноз модели."
    )

    async def mock_complete(system, payload):
        data = json.loads(payload)
        assert "MSE" in data["slide_text"]
        recovered = data["slide_text"].replace(
            "<!-- formula-not-decoded -->",
            r"$$\text{MSE} = \frac{1}{n} \sum_{i=1}^n (y_i - \hat{y}_i)^2$$"
        )
        return json.dumps({"recovered_text": recovered})

    result = asyncio.run(recover_formulas(slide, mock_complete))
    assert r"$$\text{MSE} = \frac{1}{n} \sum_{i=1}^n (y_i - \hat{y}_i)^2$$" in result
    assert "<!-- formula-not-decoded -->" not in result
    assert "## Слайд 14" in result


def test_graceful_fallback_when_llm_cannot_deduce_formula():
    slide = (
        "## Слайд 3\n\n"
        "Неизвестный график:\n\n"
        "<!-- formula-not-decoded -->"
    )

    async def mock_complete_cannot_deduce(system, payload):
        # LLM returns the slide with the placeholder intact when context is insufficient
        data = json.loads(payload)
        return json.dumps({"recovered_text": data["slide_text"]})

    result = asyncio.run(recover_formulas(slide, mock_complete_cannot_deduce))
    assert result == slide
    assert "<!-- formula-not-decoded -->" in result


def test_graceful_fallback_on_llm_exception():
    slide = (
        "## Слайд 5\n\n"
        "Формула:\n\n"
        "<!-- formula-not-decoded -->"
    )

    async def mock_failing_complete(system, payload):
        raise RuntimeError("LLM gateway unavailable")

    result = asyncio.run(recover_formulas(slide, mock_failing_complete))
    assert result == slide
    assert "<!-- formula-not-decoded -->" in result
