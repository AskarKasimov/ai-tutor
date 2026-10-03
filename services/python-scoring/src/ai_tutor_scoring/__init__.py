"""Strict JSON-in/JSON-out scoring for AI Tutor."""

from .client import LLMSettings, OpenAICompatibleGrader
from .scorer import ScoreInputError, ScoreProviderError, score

__all__ = [
    "LLMSettings",
    "OpenAICompatibleGrader",
    "ScoreInputError",
    "ScoreProviderError",
    "score",
]
