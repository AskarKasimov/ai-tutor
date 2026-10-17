import { afterEach, expect, it, vi } from 'vitest'
import {
  importCompetencyMap,
  readCompetencyMap,
  readSubjectMaterials,
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

it('reads optional material progress and OCR warnings while accepting legacy rows', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json([
        {
          id: 'active',
          subject_id: 'course',
          filename: 'lecture.pdf',
          content_type: 'application/pdf',
          file_size: 100,
          s3_raw_key: 'raw/lecture.pdf',
          s3_parsed_key: 'parsed/lecture.md',
          parsed_text: '',
          char_count: 0,
          status: 'pending',
          processing: {
            stage: 'waiting_models',
            status: 'processing',
            updated_at: 1792180000,
            attempts: 2,
          },
          ocr_report: {
            warnings: ['Некоторые изображения не распознаны.'],
            pipeline_warnings: ['Перевод не завершён.'],
            pages: 57,
          },
          created_at: 1792180000,
          updated_at: 1792180000,
        },
        {
          id: 'legacy',
          subject_id: 'course',
          filename: 'notes.md',
          content_type: 'text/markdown',
          file_size: 20,
          s3_raw_key: 'raw/notes.md',
          s3_parsed_key: 'parsed/notes.md',
          parsed_text: 'Текст',
          char_count: 5,
          status: 'parsed',
          created_at: 1792180000,
          updated_at: 1792180000,
        },
      ]),
    ),
  )

  const materials = await readSubjectMaterials('course')

  expect(materials[0]).toMatchObject({
    processing: {
      stage: 'waiting_models',
      status: 'processing',
      updatedAt: 1792180000,
      attempts: 2,
    },
    ocrWarnings: [
      'Некоторые изображения не распознаны.',
      'Перевод не завершён.',
    ],
  })
  expect(materials[1]).toMatchObject({
    status: 'parsed',
    processing: undefined,
    cleaning: undefined,
    ocrWarnings: [],
  })
})
