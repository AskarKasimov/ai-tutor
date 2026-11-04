import { afterEach, expect, it, vi } from 'vitest'
import {
  createSubject,
  listSubjects,
  readAudioPreparation,
  SubjectApiError,
} from '@/entities/subject'

const emptyPreparation = {
  exam: { state: 'empty', source: null, stale: false },
  learn: { state: 'empty' },
} as const

afterEach(() => vi.unstubAllGlobals())

it('maps invalid JSON success bodies to INVALID_RESPONSE', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('not json')))
  await expect(
    listSubjects(new AbortController().signal),
  ).rejects.toBeInstanceOf(SubjectApiError)
  await expect(
    listSubjects(new AbortController().signal),
  ).rejects.toMatchObject({
    status: 0,
    code: 'INVALID_RESPONSE',
  })
})

it('preserves cancellation when the response body read is aborted', async () => {
  const controller = new AbortController()
  const response = new Response('ignored')
  vi.spyOn(response, 'json').mockImplementation(async () => {
    controller.abort()
    throw new SyntaxError('body read interrupted')
  })
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
  await expect(listSubjects(controller.signal)).rejects.toMatchObject({
    name: 'AbortError',
  })
})

it('lists subjects with cookie credentials and rejects malformed success bodies', async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    Response.json([
      {
        id: 'subject:ml',
        name: 'ML',
        ready: true,
        learning_ready: true,
        exam_ready: true,
        preparation: {
          exam: { state: 'ready', source: 'materials', stale: false },
          learn: { state: 'ready' },
        },
      },
    ]),
  )
  vi.stubGlobal('fetch', fetchMock)
  await expect(listSubjects(new AbortController().signal)).resolves.toEqual([
    {
      id: 'subject:ml',
      name: 'ML',
      ready: true,
      learning_ready: true,
      exam_ready: true,
      preparation: {
        exam: { state: 'ready', source: 'materials', stale: false },
        learn: { state: 'ready' },
      },
    },
  ])
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/exam/subjects')
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json([{ id: 1 }])))
  await expect(
    listSubjects(new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('creates a subject as JSON and carries backend error codes', async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    Response.json(
      {
        id: 's1',
        name: 'Физика',
        ready: false,
        learning_ready: false,
        exam_ready: false,
        preparation: emptyPreparation,
      },
      { status: 201 },
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    createSubject('Физика', new AbortController().signal),
  ).resolves.toMatchObject({ id: 's1', ready: false })
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/exam/admin/subjects')
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

it('requires a complete consistent preparation snapshot for admin catalogs', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json([
        {
          id: 's1',
          name: 'Курс',
          ready: false,
          learning_ready: false,
          exam_ready: false,
        },
      ]),
    ),
  )
  await expect(
    listSubjects(new AbortController().signal, true),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json([
        {
          id: 's1',
          name: 'Курс',
          ready: false,
          learning_ready: false,
          exam_ready: false,
          preparation: emptyPreparation,
        },
      ]),
    ),
  )
  await expect(
    listSubjects(new AbortController().signal, true),
  ).resolves.toHaveLength(1)
})

it('reads both audio snapshots and rejects mismatched counter totals', async () => {
  const direction = {
    version: 'revision:3',
    state: 'queued',
    total: 2,
    unchecked: 0,
    queued: 1,
    processing: 0,
    retry_wait: 0,
    ready: 1,
    failed: 0,
    unavailable: 0,
    observed_at: '2026-11-04T10:00:00Z',
  }
  const fetchMock = vi.fn().mockResolvedValue(
    Response.json({
      subject_id: 's1',
      exam: { ...direction, version: 'graph|voice', published: true },
      learn: { ...direction, revision: 3 },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readAudioPreparation('s1', new AbortController().signal),
  ).resolves.toMatchObject({
    subject_id: 's1',
    exam: { published: true, total: 2, ready: 1 },
    learn: { revision: 3, total: 2, queued: 1 },
  })
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/exam/admin/subjects/s1/audio-preparation',
  )
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json({
        subject_id: 's1',
        exam: { ...direction, total: 3, published: true },
        learn: { ...direction, revision: 3 },
      }),
    ),
  )
  await expect(
    readAudioPreparation('s1', new AbortController().signal),
  ).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
