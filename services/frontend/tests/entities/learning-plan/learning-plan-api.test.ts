import { afterEach, expect, it, vi } from 'vitest'
import {
  answerLesson,
  createLearningPlan,
  findLearningPlan,
  LearningPlanApiError,
  openLesson,
  retryLessonAnswer,
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

it('allows a retry to finish three sequential teacher calls', async () => {
  vi.useFakeTimers()
  const timeout = vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) => {
    const controller = new AbortController()
    setTimeout(
      () => controller.abort(new DOMException('Timed out', 'TimeoutError')),
      ms,
    )
    return controller.signal
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(Response.json(plan)), 210_000)
        }),
    ),
  )

  const request = retryLessonAnswer('p1', signal())
  await vi.advanceTimersByTimeAsync(210_000)
  await expect(request).resolves.toEqual(plan)
  expect(timeout).toHaveBeenCalledWith(expect.any(Number))
  expect(timeout.mock.calls[0][0]).toBeGreaterThanOrEqual(210_000)
  vi.useRealTimers()
})

it('gives opening theory and practice a complete operation budget', async () => {
  vi.useFakeTimers()
  const timeout = vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) => {
    const controller = new AbortController()
    setTimeout(
      () => controller.abort(new DOMException('Timed out', 'TimeoutError')),
      ms,
    )
    return controller.signal
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(Response.json(plan)), 300_000)
        }),
    ),
  )

  const request = openLesson('p1', signal())
  await vi.advanceTimersByTimeAsync(300_000)
  await expect(request).resolves.toEqual(plan)
  expect(timeout.mock.calls[0][0]).toBeGreaterThanOrEqual(300_000)
  vi.useRealTimers()
})

it('allows legacy voice upload plus teacher evaluation to finish', async () => {
  vi.useFakeTimers()
  const timeout = vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) => {
    const controller = new AbortController()
    setTimeout(
      () => controller.abort(new DOMException('Timed out', 'TimeoutError')),
      ms,
    )
    return controller.signal
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(Response.json(plan)), 570_000)
        }),
    ),
  )

  const request = answerLesson(
    'p1',
    new Blob(['voice'], { type: 'audio/webm' }),
    signal(),
  )
  await vi.advanceTimersByTimeAsync(570_000)
  await expect(request).resolves.toEqual(plan)
  expect(timeout.mock.calls[0][0]).toBeGreaterThanOrEqual(570_000)
  vi.useRealTimers()
})

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
  expect(url).toBe('/api/v1/learn/diagnostic-sessions/d1/learning-plan')
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

it('sends the confirmed transcript and task identity without uploading audio again', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json(plan))
  vi.stubGlobal('fetch', fetcher)
  const confirmed = {
    transcription_id: 'preview-id',
    item_index: 0,
    attempts: 0,
    question: 'Вопрос?',
  }
  await answerLesson('p1', confirmed, new AbortController().signal)
  expect(fetcher.mock.calls[0][0]).toBe(
    '/api/v1/learn/learning-plans/p1/answers/transcription',
  )
  expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual(confirmed)
  expect(fetcher.mock.calls[0][1].headers).toEqual({
    'Content-Type': 'application/json',
  })
})
