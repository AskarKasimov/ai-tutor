import { afterEach, expect, it, vi } from 'vitest'
import {
  readCourseGraph,
  readLatestCourseGraphBuild,
  startCourseGraphBuild,
} from '@/entities/course-graph'

afterEach(() => vi.unstubAllGlobals())

const signal = () => new AbortController().signal

const build = (status: 'queued' | 'running' | 'succeeded' | 'failed') => ({
  id: 'build-1',
  subject_id: 'subject-1',
  status,
  stage: status === 'running' ? 'units' : null,
  started_at: status === 'queued' ? null : 10,
  completed_at: ['failed', 'succeeded'].includes(status) ? 20 : null,
  error:
    status === 'failed'
      ? { code: 'GRAPH_BUILD_FAILED', message: 'Не удалось собрать граф.' }
      : null,
})

it.each(['queued', 'running', 'succeeded', 'failed'] as const)(
  'parses the %s build state',
  async (status) => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(Response.json(build(status), { status: 202 })),
    )
    await expect(
      startCourseGraphBuild('subject-1', signal()),
    ).resolves.toMatchObject({
      id: 'build-1',
      status,
      error: status === 'failed' ? { code: 'GRAPH_BUILD_FAILED' } : null,
    })
  },
)

it('starts a build with cookies and reads 404 as no prior build', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(Response.json(build('queued'), { status: 202 }))
    .mockResolvedValueOnce(
      Response.json(
        { code: 'NOT_FOUND', message: 'No build' },
        { status: 404 },
      ),
    )
  vi.stubGlobal('fetch', fetchMock)
  await startCourseGraphBuild('subject:one', signal())
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/admin/subjects/subject%3Aone/graph/build',
  )
  expect(fetchMock.mock.calls[0][1]).toMatchObject({
    method: 'POST',
    credentials: 'include',
  })
  await expect(
    readLatestCourseGraphBuild('subject:one', signal()),
  ).resolves.toBeNull()
})

it.each([
  [409, 'GRAPH_BUILD_ACTIVE'],
  [422, 'VALIDATION_ERROR'],
])('preserves structured %i errors', async (status, code) => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ code, message: 'Backend error' }, { status }),
      ),
  )
  await expect(
    startCourseGraphBuild('subject-1', signal()),
  ).rejects.toMatchObject({
    status,
    code,
    message: 'Backend error',
  })
})

it('reads the graph and rejects malformed success payloads', async () => {
  const graph = {
    version: 'build-1',
    published_at: 42,
    nodes: [
      {
        id: 'topic-1',
        title: 'Матрицы',
        content: 'Таблица чисел.',
        prerequisites: [],
        outcomes: [],
        questions: [],
      },
    ],
  }
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(graph)))
  await expect(readCourseGraph('subject-1', signal())).resolves.toMatchObject({
    version: 'build-1',
    publishedAt: 42,
  })
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json({ version: 2, nodes: [] })),
  )
  await expect(readCourseGraph('subject-1', signal())).rejects.toMatchObject({
    code: 'INVALID_RESPONSE',
  })
})
