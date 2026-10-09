import { afterEach, expect, it, vi } from 'vitest'
import {
  importCompetencyMap,
  readCompetencyMap,
} from '@/entities/competency-map'

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
    '/api/v1/subjects/subject%3Aa%2Fb/competency-map',
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
    '/api/v1/admin/subjects/subject%3Aa%2Fb/competency-map/import',
  )
  expect(fetchMock.mock.calls[0][1].method).toBe('POST')
  expect(
    (fetchMock.mock.calls[0][1].body as FormData).get('file'),
  ).toMatchObject({ name: 'x.csv', type: 'text/csv' })
})
