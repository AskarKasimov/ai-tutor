import { afterEach, expect, it, vi } from 'vitest'

import { evaluateAnswer } from './assessment-api'

afterEach(() => vi.unstubAllGlobals())

it('sends the task and transcription id to the backend and accepts a three-line grade', async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ score: 2, feedback: ['Верно.', 'Два класса.', 'Закрепите тему.'] }) })
  vi.stubGlobal('fetch', fetchMock)
  const task = { question: 'Вопрос', options: ['Классификация', 'Регрессия'], voiceInstruction: 'Назовите тип', correctAnswer: 'Классификация' }
  const grade = await evaluateAnswer('tr-1', task, new AbortController().signal)
  expect(grade.score).toBe(2)
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/v1/assessments/evaluate')
  expect(init.credentials).toBe('include')
  expect(JSON.parse(init.body)).toEqual({ transcription_id: 'tr-1', question: 'Вопрос', options: task.options, voice_instruction: 'Назовите тип', correct_answer: 'Классификация' })
})

it('rejects invalid model output rather than displaying a fabricated score', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ score: 7, feedback: ['Некорректно.'] }) }))
  await expect(evaluateAnswer('tr-1', { question: 'Вопрос', options: [], voiceInstruction: 'Ответьте' }, new AbortController().signal)).rejects.toThrow('Invalid assessment response')
})
