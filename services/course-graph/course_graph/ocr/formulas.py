"""Recover undecoded OCR formulas via LLM context inference."""
import json
import logging
import re
from typing import Awaitable, Callable

logger = logging.getLogger(__name__)

RECOVER_FORMULA_PROMPT = """Ты восстанавливаешь математические формулы, которые OCR не смог распознать в слайде лекции.
Входной фрагмент содержит маркер <!-- formula-not-decoded -->.
Изучи контекст: названия алгоритмов, определений, переменных и пояснений.
Если математическое выражение однозначно понятно из контекста курса (машинное обучение, Computer Science, логика, теория вероятностей, дискретная математика),
замени маркер <!-- formula-not-decoded --> на корректную LaTeX-формулу ($...$ для формулы в строке или $$...$$ для выносной строки).
Если контекста недостаточно, чтобы уверенно восстановить точную формулу, верни фрагмент с <!-- formula-not-decoded --> без изменений.
Не удаляй текст, не меняй названия слайдов и другие слова.
Верни только JSON вида: {"recovered_text": "полный текст фрагмента с подставленной LaTeX формулой"}
"""


async def recover_formulas(markdown: str, complete: Callable[[str, str], Awaitable[str]]) -> str:
    """Scan markdown for undecoded formula markers and reconstruct LaTeX math via LLM."""
    if "<!-- formula-not-decoded -->" not in markdown:
        return markdown

    # Split markdown by slide headings to process each slide with its full local context
    slides = re.split(r"(?m)(?=^##\s+(?:Слайд|Slide|Страница|Page)\s+\d+)", markdown)
    recovered_slides = []

    for slide in slides:
        if "<!-- formula-not-decoded -->" not in slide:
            recovered_slides.append(slide)
            continue

        try:
            payload = json.dumps({"slide_text": slide}, ensure_ascii=False)
            raw = await complete(RECOVER_FORMULA_PROMPT, payload)
            clean_raw = raw.strip()
            if clean_raw.startswith("```"):
                clean_raw = re.sub(r"^```(?:json)?\s*", "", clean_raw)
                clean_raw = re.sub(r"\s*```$", "", clean_raw)

            data = json.loads(clean_raw)
            recovered = data.get("recovered_text")
            if recovered and isinstance(recovered, str):
                # Sanity guard: do not lose slide headings if the slide had one
                has_orig_heading = bool(re.search(r"(?m)^##\s+", slide))
                has_new_heading = bool(re.search(r"(?m)^##\s+", recovered))
                if has_orig_heading == has_new_heading and len(recovered.strip()) > 10:
                    recovered_slides.append(recovered)
                    continue

            recovered_slides.append(slide)
        except Exception as error:
            logger.warning("Formula recovery failed for slide: %s", error)
            recovered_slides.append(slide)

    return "".join(recovered_slides)
