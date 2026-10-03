"""OpenAI-compatible transport adapter for the scoring module."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Optional, Protocol
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


class Grader(Protocol):
    """Returns the raw JSON object produced by the LLM."""

    def grade(self, system_prompt: str, context_json: str) -> str: ...


class ScoreProviderError(RuntimeError):
    """A retryable or invalid upstream LLM response."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class LLMSettings:
    base_url: str
    model: str
    timeout_seconds: float = 90.0
    api_key: Optional[str] = None

    @classmethod
    def from_env(cls) -> "LLMSettings":
        base_url = os.getenv("ASSESSMENT_BASE_URL", "http://10.100.10.105:30245/v1").rstrip("/")
        model = os.getenv("ASSESSMENT_MODEL", "gpt-oss-120b")
        try:
            timeout_seconds = float(os.getenv("ASSESSMENT_TIMEOUT_SECONDS", "90"))
        except ValueError as error:
            raise ValueError("ASSESSMENT_TIMEOUT_SECONDS must be numeric") from error
        if not base_url.startswith(("http://", "https://")) or not model or timeout_seconds <= 0:
            raise ValueError("invalid assessment model configuration")
        return cls(base_url, model, timeout_seconds, os.getenv("ASSESSMENT_API_KEY") or None)


@dataclass
class OpenAICompatibleGrader:
    settings: LLMSettings

    def grade(self, system_prompt: str, context_json: str) -> str:
        body = json.dumps(
            {
                "model": self.settings.model,
                "messages": [
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": context_json},
                ],
                "response_format": {"type": "json_object"},
                "temperature": 0,
                "max_tokens": 1024,
            },
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode("utf-8")
        request = Request(
            f"{self.settings.base_url}/chat/completions",
            data=body,
            headers=self._headers(),
            method="POST",
        )
        try:
            with urlopen(request, timeout=self.settings.timeout_seconds) as response:
                content = response.read(65_537)
        except HTTPError as error:
            code = "ASSESSMENT_TIMEOUT" if error.code in {408, 504} else "ASSESSMENT_UNAVAILABLE" if error.code in {429, 503} else "ASSESSMENT_FAILED"
            raise ScoreProviderError(code, "LLM request was rejected") from error
        except (TimeoutError, URLError) as error:
            raise ScoreProviderError("ASSESSMENT_TIMEOUT" if isinstance(error, TimeoutError) else "ASSESSMENT_UNAVAILABLE", "LLM is unavailable") from error
        if len(content) > 65_536:
            raise ScoreProviderError("ASSESSMENT_FAILED", "LLM response is too large")
        try:
            completion = json.loads(content)
            choices = completion["choices"]
            choice = choices[0]
            if len(choices) != 1 or choice.get("finish_reason") != "stop":
                raise ValueError("incomplete completion")
            result = choice["message"]["content"]
            if not isinstance(result, str):
                raise ValueError("missing completion content")
            return result
        except (KeyError, IndexError, TypeError, ValueError, json.JSONDecodeError) as error:
            raise ScoreProviderError("ASSESSMENT_FAILED", "LLM returned an invalid completion") from error

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json", "Accept": "application/json"}
        if self.settings.api_key:
            headers["Authorization"] = f"Bearer {self.settings.api_key}"
        return headers
