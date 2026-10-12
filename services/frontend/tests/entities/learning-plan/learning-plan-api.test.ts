import { afterEach, expect, it, vi } from 'vitest'
import {
  answerLesson,
  createLearningPlan,
  findLearningPlan,
  LearningPlanApiError,
} from '@/entities/learning-plan'

afterEach(() => vi.unstubAllGlobals())

const plan = {
  id: 'p1',
  diagnostic_session_id: 'd1',
  subject_id: 's1',
  subject_name: 'ML',
  hours: 3,
  detail: 'standard',
  status: 'active',
  current_index: 0,
  progress_percent: 0,
  planned_minutes: 30,
  pending_answer: null,
  items: [
    {
      index: 0,
      competency_id: 'c1',
      competency_name: 'Регрессия',
      outcome_id: 'o1',
      outcome_name: 'Знает параметры',
      theory_minutes: 15,
      practice_minutes: 15,
      in_plan: true,
      status: 'current',
      attempts: 0,
    },
  ],
}
const signal = () => new AbortController().signal

it('reads no plan before the hours are chosen', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ code: 'LEARNING_PLAN_NOT_FOUND' }, { status: 404 }),
      ),
  )
  await expect(findLearningPlan('d1', signal())).resolves.toBeNull()
})

it('creates the plan with the chosen hours', async () => {
  const fetchMock = vi.fn().mockResolvedValue(Response.json(plan))
  vi.stubGlobal('fetch', fetchMock)
  await expect(createLearningPlan('d1', 3, signal())).resolves.toEqual(plan)
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
  expect(url).toBe('/api/v1/diagnostic-sessions/d1/learning-plan')
  expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
  expect(JSON.parse(String(init.body))).toEqual({ hours: 3 })
})

it('sends a voice answer as multipart audio and keeps API error codes', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(Response.json(plan))
    .mockResolvedValueOnce(
      Response.json({ code: 'NO_SPEECH_DETECTED' }, { status: 422 }),
    )
  vi.stubGlobal('fetch', fetchMock)
  const audio = new Blob(['voice'], { type: 'audio/webm' })
  await answerLesson('p1', audio, signal())
  const body = (fetchMock.mock.calls[0] as [string, RequestInit])[1]
    .body as FormData
  expect((body.get('audio') as File).name).toBe('answer.webm')
  await expect(answerLesson('p1', audio, signal())).rejects.toMatchObject({
    status: 422,
    code: 'NO_SPEECH_DETECTED',
  })
})

it('rejects a malformed plan', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json({ ...plan, items: 'broken' })),
  )
  await expect(createLearningPlan('d1', 1, signal())).rejects.toBeInstanceOf(
    LearningPlanApiError,
  )
})
