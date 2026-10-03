import json
import unittest
from unittest.mock import patch
from urllib.error import HTTPError

from ai_tutor_scoring.client import LLMSettings, OpenAICompatibleGrader, ScoreProviderError


class Response:
    def __init__(self, body: bytes) -> None:
        self.body = body

    def read(self, _limit: int) -> bytes:
        return self.body

    def __enter__(self) -> "Response":
        return self

    def __exit__(self, *_args: object) -> None:
        return None


class OpenAICompatibleGraderTests(unittest.TestCase):
    def setUp(self) -> None:
        self.grader = OpenAICompatibleGrader(LLMSettings("http://llm.internal/v1", "gpt-oss-120b", 2))

    @patch("ai_tutor_scoring.client.urlopen")
    def test_sends_json_request_and_extracts_single_completion(self, urlopen_mock) -> None:
        urlopen_mock.return_value = Response(
            b'{"choices":[{"message":{"content":"{\\"mark\\":2}"},"finish_reason":"stop"}]}'
        )

        self.assertEqual(self.grader.grade("system", '{"student_answer":"ответ"}'), '{"mark":2}')
        request = urlopen_mock.call_args.args[0]
        body = json.loads(request.data)
        self.assertEqual(request.full_url, "http://llm.internal/v1/chat/completions")
        self.assertEqual(body["model"], "gpt-oss-120b")
        self.assertEqual(body["temperature"], 0)
        self.assertEqual(body["messages"][1]["content"], '{"student_answer":"ответ"}')

    @patch("ai_tutor_scoring.client.urlopen")
    def test_maps_rate_limit_to_unavailable(self, urlopen_mock) -> None:
        urlopen_mock.side_effect = HTTPError("http://llm", 429, "limited", {}, None)

        with self.assertRaises(ScoreProviderError) as raised:
            self.grader.grade("system", "{}")
        self.assertEqual(raised.exception.code, "ASSESSMENT_UNAVAILABLE")

    @patch("ai_tutor_scoring.client.urlopen")
    def test_rejects_truncated_completion(self, urlopen_mock) -> None:
        urlopen_mock.return_value = Response(b'{"choices":[{"message":{"content":null},"finish_reason":"length"}]}')

        with self.assertRaises(ScoreProviderError) as raised:
            self.grader.grade("system", "{}")
        self.assertEqual(raised.exception.code, "ASSESSMENT_FAILED")


if __name__ == "__main__":
    unittest.main()
