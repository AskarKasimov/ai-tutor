import { afterEach, expect, it, vi } from 'vitest'
import {
  readTrainingChat,
  sendTrainingChatText,
  createTrainingChatText,
  TrainingApiError,
} from '@/entities/training'

afterEach(() => vi.unstubAllGlobals())

const message = (
  id: string,
  sequence: number,
  role: string,
  kind = 'message',
) => ({
  id,
  sequence,
  role,
  kind,
  text: 'Let us reason together.',
  created_at: 10,
})
const check = {
  revision: 0,
  final_answer: 'answer',
  source: 'text',
  score: 1,
  max_score: 2,
  verdict: 'partial',
}
const page = (overrides: Record<string, unknown> = {}) => ({
  session_id: 's1',
  exercise_id: 'e1',
  revision: 0,
  status: 'active',
  items: [message('m1', 1, 'assistant', 'welcome')],
  next_cursor: null,
  latest_check: null,
  pending_turn: null,
  ...overrides,
})

it('reads chat with cursor and validates ownership tuple', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      Response.json(page({ session_id: 's/1', exercise_id: 'e 1' })),
    )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readTrainingChat('s/1', 'e 1', '3', 40, new AbortController().signal),
  ).resolves.toMatchObject({ session_id: 's/1' })
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/training-sessions/s%2F1/exercises/e%201/chat?limit=40&cursor=3',
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(page({ exercise_id: 'wrong' }))),
  )
  await expect(
    readTrainingChat('s1', 'e1', undefined, 40, new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('rejects unknown roles, duplicate sequences, and private grading fields', async () => {
  for (const body of [
    page({ items: [message('m1', 1, 'system')] }),
    page({ items: [message('m1', 1, 'user'), message('m2', 1, 'assistant')] }),
    page({ reference_answer: 'secret' }),
  ]) {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(body)))
    await expect(
      readTrainingChat('s1', 'e1', undefined, 40, new AbortController().signal),
    ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  }
})

it('sends a text check with only the public input and validates the turn pair', async () => {
  const result = {
    session_id: 's1',
    exercise_id: 'e1',
    revision: 1,
    turn_id: 'turn1',
    messages: [
      message('u1', 2, 'user', 'check'),
      message('a1', 3, 'assistant', 'check'),
    ],
    latest_check: { ...check, revision: 1 },
  }
  const fetchMock = vi.fn().mockResolvedValue(Response.json(result))
  vi.stubGlobal('fetch', fetchMock)
  await sendTrainingChatText(
    's1',
    'e1',
    createTrainingChatText('key', 0, 'check', 'answer'),
    new AbortController().signal,
  )
  const init = fetchMock.mock.calls[0][1]
  expect(JSON.parse(init.body)).toEqual({
    revision: 0,
    kind: 'check',
    text: 'answer',
  })
  expect(init.headers).toMatchObject({ 'Idempotency-Key': 'key' })
  expect(init.body).not.toContain('reference_answer')

  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ ...result, latest_check: { ...check, revision: 0 } }),
      ),
  )
  await expect(
    sendTrainingChatText(
      's1',
      'e1',
      createTrainingChatText('key', 0, 'check', 'answer'),
      new AbortController().signal,
    ),
  ).rejects.toBeInstanceOf(TrainingApiError)
})
