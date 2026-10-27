import { afterEach, expect, it, vi } from 'vitest'
import {
  importCompetencyMap,
  readCompetencyMap,
  readCompetencyMapOutline,
} from '@/entities/learning-competency-map'

afterEach(() => vi.unstubAllGlobals())

it('reads a map from the encoded subject resource URL', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      Response.json({ revision: 0, imported_at: null, competencies: [] }),
    )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    readCompetencyMap('subject:a/b', new AbortController().signal),
  ).resolves.toMatchObject({ importedAt: null })
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/learn/subjects/subject%3Aa%2Fb/competency-map',
  )
})

it('imports into the explicit encoded subject and preserves multipart file data', async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    Response.json({
      revision: 1,
      imported_at: 2,
      competency_count: 0,
      constituent_count: 0,
      outcome_count: 0,
      task_count: 0,
      unparsed_task_cell_count: 0,
      warnings: [],
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  const file = new File(['map'], 'x.csv')
  await importCompetencyMap('subject:a/b', file)
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/learn/admin/subjects/subject%3Aa%2Fb/competency-map/import',
  )
  expect(fetchMock.mock.calls[0][1].method).toBe('POST')
  expect(
    (fetchMock.mock.calls[0][1].body as FormData).get('file'),
  ).toMatchObject({ name: 'x.csv', type: 'text/csv' })
})

it('keeps only TRUE outcomes in map order and removes empty topics and competencies', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json({
        competencies: [
          {
            id: 'c1',
            name: 'Первая',
            constituents: [
              {
                id: 'empty',
                name: 'Пустая',
                outcomes: [{ id: 'no', name: 'Нет', include_in_test: false }],
              },
              {
                id: 't1',
                name: 'Тема',
                outcomes: [
                  {
                    id: 'o1',
                    name: 'Первый',
                    include_in_test: true,
                    tasks: [{ question: 'Не показывать вопрос' }],
                  },
                  { id: 'null', name: 'Не включён', include_in_test: null },
                  { id: 'o2', name: 'Второй', include_in_test: true },
                ],
              },
            ],
          },
          { id: 'c2', name: 'Пустая', constituents: [] },
        ],
      }),
    ),
  )
  await expect(
    readCompetencyMapOutline('course/a', new AbortController().signal),
  ).resolves.toEqual({
    competencies: [
      {
        id: 'c1',
        name: 'Первая',
        constituents: [
          {
            id: 't1',
            name: 'Тема',
            outcomes: [
              { id: 'o1', name: 'Первый' },
              { id: 'o2', name: 'Второй' },
            ],
          },
        ],
      },
    ],
  })
  expect(vi.mocked(fetch).mock.calls[0][0]).toBe(
    '/api/v1/learn/subjects/course%2Fa/competency-map',
  )
})

it('reports malformed map data instead of showing an empty plan', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(Response.json({ competencies: [{ id: 'broken' }] })),
  )
  await expect(
    readCompetencyMapOutline('course', new AbortController().signal),
  ).rejects.toThrow()
})
