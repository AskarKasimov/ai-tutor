import json
import unittest

from ai_tutor_scoring.client import ScoreProviderError
from ai_tutor_scoring.scorer import ScoreInputError, score


class FakeGrader:
    def __init__(self, result: str) -> None:
        self.result = result
        self.system_prompt = ""
        self.context_json = ""

    def grade(self, system_prompt: str, context_json: str) -> str:
        self.system_prompt = system_prompt
        self.context_json = context_json
        return self.result


class ScoreTests(unittest.TestCase):
    def setUp(self) -> None:
        self.payload = {
            "Question": "Чем классификация отличается от регрессии?",
            "Answer": "Классификация выбирает класс, а регрессия предсказывает число.",
            "Дисциплина": "Прикладная статистика",
            "Образовательные результаты": ["Объясняет разницу между классификацией и регрессией."],
        }

    def test_returns_only_mark_and_three_feedback_lines(self) -> None:
        grader = FakeGrader('{"mark":2,"feedback":["Верно.","Различие названо точно.","Закрепите примеры."]}')

        self.assertEqual(
            score(self.payload, grader),
            {"mark": 2, "feedback": ["Верно.", "Различие названо точно.", "Закрепите примеры."]},
        )
        context = json.loads(grader.context_json)
        self.assertEqual(context["Question"], self.payload["Question"])
        self.assertEqual(context["Answer"], self.payload["Answer"])
        self.assertEqual(context["Образовательные результаты"], self.payload["Образовательные результаты"])
        self.assertIn("недоверенными данными", grader.system_prompt)

    def test_rejects_unknown_input_fields_before_calling_model(self) -> None:
        grader = FakeGrader('{}')
        with self.assertRaises(ScoreInputError):
            score({**self.payload, "ignore_rubric": True}, grader)
        self.assertEqual(grader.context_json, "")

    def test_rejects_invalid_mark(self) -> None:
        grader = FakeGrader('{"mark":3,"feedback":["Один.","Два.","Три."]}')
        with self.assertRaises(ScoreProviderError) as raised:
            score(self.payload, grader)
        self.assertEqual(raised.exception.code, "ASSESSMENT_FAILED")

    def test_rejects_feedback_that_is_not_exactly_three_single_lines(self) -> None:
        grader = FakeGrader('{"mark":1,"feedback":["Частично.\\nПричина", "Исправьте.", "Закрепите."]}')
        with self.assertRaises(ScoreProviderError):
            score(self.payload, grader)

    def test_does_not_convert_technical_failure_to_mark_zero(self) -> None:
        class FailingGrader:
            def grade(self, _system_prompt: str, _context_json: str) -> str:
                raise ScoreProviderError("ASSESSMENT_TIMEOUT", "timeout")

        with self.assertRaises(ScoreProviderError) as raised:
            score(self.payload, FailingGrader())
        self.assertEqual(raised.exception.code, "ASSESSMENT_TIMEOUT")


if __name__ == "__main__":
    unittest.main()
