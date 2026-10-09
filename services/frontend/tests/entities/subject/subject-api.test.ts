import { afterEach, expect, it, vi } from 'vitest'
import {
  createSubject,
  listSubjects,
  readLearningState,
} from '@/entities/subject'

afterEach(() => vi.unstubAllGlobals())

it('lists subjects with cookie credentials and rejects malformed success bodies', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      Response.json([{ id: 'subject:ml', name: 'ML', ready: true }]),
    )
  vi.stubGlobal('fetch', fetchMock)
  await expect(listSubjects(new AbortController().signal)).resolves.toEqual([
    { id: 'subject:ml', name: 'ML', ready: true },
  ])
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/subjects')
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json([{ id: 1 }])))
  await expect(
    listSubjects(new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('creates a subject as JSON and carries backend error codes', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      Response.json(
        { id: 's1', name: 'Физика', ready: false },
        { status: 201 },
      ),
    )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    createSubject('Физика', new AbortController().signal),
  ).resolves.toMatchObject({ id: 's1', ready: false })
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/admin/subjects')
  expect(fetchMock.mock.calls[0][1].method).toBe('POST')
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
    name: 'Физика',
  })
  expect(fetchMock.mock.calls[0][1].headers).toMatchObject({
    'Content-Type': 'application/json',
  })
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(Response.json({ code: 'CONFLICT' }, { status: 409 })),
  )
  await expect(
    createSubject('Физика', new AbortController().signal),
  ).rejects.toMatchObject({ status: 409, code: 'CONFLICT' })
})

it('reads encoded learning state and checks the requested subject ID', async () => {
  const state = {
    subject_id: 'subject:ml/a',
    subject_name: 'ML',
    diagnostic_status: 'completed',
    diagnostic_completed: true,
    training_available: true,
    diagnostic_session_id: 'd1',
    active_session_id: 't1',
  }
  const fetchMock = vi.fn().mockResolvedValue(Response.json(state))
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readLearningState('subject:ml/a', new AbortController().signal),
  ).resolves.toEqual(state)
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/subjects/subject%3Aml%2Fa/learning-state',
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json({ ...state, subject_id: 'other' })),
  )
  await expect(
    readLearningState('subject:ml/a', new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
