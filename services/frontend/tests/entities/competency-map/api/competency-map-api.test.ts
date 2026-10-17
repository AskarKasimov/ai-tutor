import { afterEach, expect, it, vi } from 'vitest'
import {
  readSubjectMaterials,
  uploadCourseMaterials,
  requestCourseGraphBuild,
} from '@/entities/competency-map'
afterEach(() => vi.unstubAllGlobals())
const material = {
  id: 'material-1',
  subject_id: 'subject-1',
  filename: 'lecture.pdf',
  content_type: 'application/pdf',
  file_size: 4,
  s3_raw_key: 'raw',
  s3_parsed_key: 'parsed',
  parsed_text: 'Текст',
  char_count: 5,
  status: 'parsed',
  created_at: 1,
  updated_at: 1,
}
it('reads pipeline stages and warnings without requiring legacy metadata', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json([
        {
          ...material,
          processing: {
            stage: 'translation',
            status: 'processing',
            updated_at: 2,
            attempts: 1,
          },
          ocr_report: { warnings: ['Низкое качество'] },
        },
      ]),
    ),
  )
  await expect(readSubjectMaterials('subject-1')).resolves.toMatchObject([
    { processing: { stage: 'translation' }, ocrWarnings: ['Низкое качество'] },
  ])
})
it('sends ordered files as repeated multipart file fields', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ uploaded: [material], failed: [] }))
  vi.stubGlobal('fetch', fetch)
  await uploadCourseMaterials('subject-1', [
    new File(['%PDF-one'], 'one.pdf'),
    new File(['%PDF-two'], 'two.pdf'),
  ])
  const body = fetch.mock.calls[0][1].body as FormData
  expect([...body.keys()]).toEqual(['file', 'file'])
  expect((body.getAll('file') as File[]).map((file) => file.name)).toEqual([
    'one.pdf',
    'two.pdf',
  ])
})
it('starts a graph build through the master snapshot endpoint', async () => {
  const fetch = vi.fn().mockResolvedValue(
    Response.json({
      status: 'pending',
      attempts: 1,
      error_message: '',
      graph: null,
      stale: false,
      inputs_ready: true,
      updated_at: 2,
    }),
  )
  vi.stubGlobal('fetch', fetch)
  await expect(requestCourseGraphBuild('subject-1')).resolves.toMatchObject({
    status: 'pending',
    inputsReady: true,
  })
  expect(fetch.mock.calls[0][0]).toBe(
    '/api/v1/admin/subjects/subject-1/graph/build',
  )
  expect(fetch.mock.calls[0][1].method).toBe('POST')
})
