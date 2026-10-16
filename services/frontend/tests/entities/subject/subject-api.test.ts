import { afterEach, expect, it, vi } from 'vitest'
import {
  createSubject,
  listSubjects,
  SubjectApiError,
} from '@/entities/subject'

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
