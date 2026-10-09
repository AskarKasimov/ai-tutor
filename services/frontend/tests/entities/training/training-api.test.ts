import { afterEach, expect, it, vi } from 'vitest'
import {
  createTrainingSubmission,
  findTrainingForDiagnostic,
  readTrainingHistory,
  readTrainingAudio,
  readTrainingPreview,
  readTrainingSession,
  startTraining,
  submitTraining,
} from '@/entities/training'

afterEach(() => vi.unstubAllGlobals())

const target = {
  kind: 'confirmed_gap',
  label: 'Gaps',
  competency_id: 'c1',
  competency_name: 'C',
  outcome_id: 'o1',
  outcome_name: 'O',
  original_score: null,
  original_max_score: null,
  last_score: null,
}
const progress = (overrides: Record<string, unknown> = {}) => ({
  session_id: 't1',
  diagnostic_session_id: 'd1',
  subject_id: 's1',
  subject_name: 'ML',
  mode: 'focused',
  status: 'active',
  round: 1,
  answer_count: 0,
  targets: [target],
  current: {
    exercise_id: 'e1',
    outcome_id: 'o1',
    outcome_name: 'O',
    question: 'Q?',
    options: ['A', 'B'],
    voice_instruction: 'Choose.',
  },
  ...overrides,
})

it('reads a preview and accepts nullable original scores', async () => {
  const preview = {
    diagnostic_session_id: 'd1/x',
    subject_id: 's1',
    subject_name: 'ML',
    mode: 'focused',
    plan_revision: 7,
    diagnostic_score: 1,
    maximum_score: 2,
    status: 'ready',
    confirmed_gaps: [target],
    partial_competencies: [],
    topics: [],
  }
  const fetchMock = vi.fn().mockResolvedValue(Response.json(preview))
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readTrainingPreview('d1/x', new AbortController().signal),
  ).resolves.toMatchObject({ confirmed_gaps: [{ original_score: null }] })
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/diagnostic-sessions/d1%2Fx/training/preview',
  )
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ ...preview, mode: 'private_snapshot' }),
      ),
  )
  await expect(
    readTrainingPreview('d1', new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('starts training with mode-specific JSON and validates IDs', async () => {
  const fetchMock = vi.fn().mockResolvedValue(Response.json(progress()))
  vi.stubGlobal('fetch', fetchMock)
  await startTraining(
    'd1',
    'start-key',
    undefined,
    new AbortController().signal,
  )
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/diagnostic-sessions/d1/training',
  )
  expect(fetchMock.mock.calls[0][1].headers).toMatchObject({
    'Content-Type': 'application/json',
    'Idempotency-Key': 'start-key',
  })
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({})
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(Response.json(progress({ mode: 'free_practice' }))),
  )
  await startTraining('d1', 'free-key', 9, new AbortController().signal)
  expect(JSON.parse(vi.mocked(fetch).mock.calls[0][1]?.body as string)).toEqual(
    { plan_revision: 9 },
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(progress({ session_id: 'wrong' }))),
  )
  await expect(
    readTrainingSession('t1', new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('submits only audio and exercise_id and retains the idempotency key on retries', async () => {
  const blob = new Blob(['voice'], { type: 'audio/webm' })
  const input = createTrainingSubmission('t1', 'e1', blob)
  expect(input.key).toBeTruthy()
  expect([...input.body.keys()]).toEqual(['audio', 'exercise_id'])
  expect(input.body.get('exercise_id')).toBe('e1')
  const fetchMock = vi.fn().mockImplementation(() =>
    Response.json(
      progress({
        answer_count: 1,
        answer: {
          sequence: 1,
          exercise_id: 'e1',
          round: 1,
          target_index: 0,
          transcription_id: 'tr1',
          text: 'A',
          score: 2,
          max_score: 2,
          verdict: 'correct',
          criterion_results: [{ key: 'k', satisfied: true, explanation: 'ok' }],
          feedback: ['one', 'two', 'three'],
          created_at: 1,
        },
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  await submitTraining(input, new AbortController().signal)
  await submitTraining(input, new AbortController().signal)
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/training-sessions/t1/answers',
  )
  for (const [, options] of fetchMock.mock.calls) {
    expect(options?.headers).toMatchObject({ 'Idempotency-Key': input.key })
    expect(options?.credentials).toBe('include')
    expect([...(options?.body as FormData).keys()]).toEqual([
      'audio',
      'exercise_id',
    ])
  }
})

it('reads encoded session IDs and history pagination and propagates 409 codes', async () => {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Response.json(progress({ session_id: 't:1' })))
  vi.stubGlobal('fetch', fetchMock)
  await readTrainingSession('t:1', new AbortController().signal)
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/training-sessions/t%3A1')
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ items: [], targets: [target], next_cursor: 'next' }),
      ),
  )
  await expect(
    readTrainingHistory('t1', 'a b', 20, new AbortController().signal),
  ).resolves.toMatchObject({ next_cursor: 'next' })
  expect(vi.mocked(fetch).mock.calls[0][0]).toContain('cursor=a+b')
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ code: 'PLAN_CHANGED' }, { status: 409 }),
      ),
  )
  await expect(
    startTraining('d1', 'k', 1, new AbortController().signal),
  ).rejects.toMatchObject({ status: 409, code: 'PLAN_CHANGED' })
})

it('finds the active session by diagnostic ID and validates answer exercise IDs', async () => {
  const fetchMock = vi.fn().mockImplementation(() => Response.json(progress()))
  vi.stubGlobal('fetch', fetchMock)
  await findTrainingForDiagnostic('d1', new AbortController().signal)
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/diagnostic-sessions/d1/training',
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json(
        progress({
          answer_count: 1,
          answer: {
            sequence: 1,
            exercise_id: 'wrong',
            round: 1,
            target_index: 0,
            transcription_id: 'tr1',
            text: 'A',
            score: 2,
            max_score: 2,
            verdict: 'correct',
            criterion_results: [],
            feedback: ['one', 'two', 'three'],
            created_at: 1,
          },
        }),
      ),
    ),
  )
  const submission = createTrainingSubmission('t1', 'e1', new Blob(['audio']))
  await expect(
    submitTraining(submission, new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('accepts only a task-audio URL for ready training audio', async () => {
  const response = {
    exercise_id: 'e1',
    status: 'ready',
    audio_url: '/task-audio/audio_1/file',
  }
  const fetchMock = vi.fn().mockResolvedValue(Response.json(response))
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readTrainingAudio('t1', 'e1', new AbortController().signal),
  ).resolves.toEqual(response)
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/training-sessions/t1/current/audio?exercise_id=e1',
  )
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ ...response, audio_url: 'https://evil.example/a.wav' }),
      ),
  )
  await expect(
    readTrainingAudio('t1', 'e1', new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
