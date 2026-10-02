import { afterEach, expect, it, vi } from 'vitest'

import { generateTrainingTasks, listOutcomes, TaskGenerationApiError } from './task-generation-api'

afterEach(() => {
  vi.unstubAllGlobals()
})

it('lists outcomes from the competency map', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ outcomes: [{ id: 'o-1', name: 'Дроби', constituent_name: 'Сост', competency_name: 'ПК-1', task_count: 2 }] }),
  }))
  const outcomes = await listOutcomes()
  expect(outcomes).toEqual([{ id: 'o-1', name: 'Дроби', constituentName: 'Сост', competencyName: 'ПК-1', taskCount: 2 }])
})

it('generates analogous training tasks for an outcome', async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ outcome_id: 'o-1', outcome_name: 'Дроби', tasks: [{ question: 'Новый вопрос?', criteria: 'Критерий', voice_instruction: 'Ответьте голосом.', options: ['А', 'Б'] }] }),
  })
  vi.stubGlobal('fetch', fetchMock)
  const result = await generateTrainingTasks('o-1', 1)
  expect(result).toEqual({ outcomeId: 'o-1', outcomeName: 'Дроби', tasks: [{ question: 'Новый вопрос?', criteria: 'Критерий', voiceInstruction: 'Ответьте голосом.', options: ['А', 'Б'] }] })
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/v1/outcomes/o-1/training-tasks')
  expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
  expect(JSON.parse(init.body)).toEqual({ count: 1 })
})

it('surfaces HTTP errors and rejects malformed payloads', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }))
  await expect(generateTrainingTasks('o-1', 3)).rejects.toBeInstanceOf(TaskGenerationApiError)
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ outcome_id: 'o-1' }) }))
  await expect(generateTrainingTasks('o-1', 3)).rejects.toThrow('Invalid training tasks response')
})
