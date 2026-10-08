import { afterEach, expect, it, vi } from 'vitest'

import { evaluateAnswer } from './assessment-api'

afterEach(() => vi.unstubAllGlobals())

it('sends task id and transcription id and preserves structured grading result', async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({
      score: 2,
      verdict: 'correct',
      criterion_results: [
        {
          key: 'task_type',
          satisfied: true,
          explanation: 'Тип задачи назван правильно.',
        },
        {
          key: 'justification',
          satisfied: true,
          explanation: 'Выбор объяснён.',
        },
      ],
      feedback: ['Верно.', 'Оба критерия выполнены.', 'Закрепите тему.'],
    }),
  })
  vi.stubGlobal('fetch', fetchMock)

  const task = {
    taskId: 'ml_001',
    question: 'Вопрос',
    options: ['Классификация', 'Регрессия'],
    voiceInstruction: 'Назовите тип',
  }
  const grade = await evaluateAnswer('tr-1', task, new AbortController().signal)

  expect(grade.score).toBe(2)
  expect(grade.verdict).toBe('correct')
  expect(grade.criterionResults).toHaveLength(2)

  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/v1/assessments/evaluate')
  expect(init.credentials).toBe('include')
  expect(JSON.parse(init.body)).toEqual({
    transcription_id: 'tr-1',
    task_id: 'ml_001',
  })
})

it('rejects malformed structured grading response', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        score: 2,
        verdict: 'correct',
        criterion_results: [],
        feedback: ['Верно.', 'Причина.', 'Совет.'],
      }),
    }),
  )

  await expect(
    evaluateAnswer(
      'tr-1',
      {
        taskId: 'ml_001',
        question: 'Вопрос',
        options: [],
        voiceInstruction: 'Ответьте',
      },
      new AbortController().signal,
    ),
  ).rejects.toThrow('Invalid assessment response')
})
