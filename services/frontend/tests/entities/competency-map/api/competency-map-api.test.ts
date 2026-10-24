import { afterEach, expect, it, vi } from 'vitest'
import {
  readCourseMaterialKinds,
  readSubjectMaterials,
  replaceCourseMaterialFile,
  updateCourseMaterialMetadata,
  uploadCourseMaterials,
} from '@/entities/competency-map'

afterEach(() => vi.unstubAllGlobals())

it('reads configurable material kinds without a client-side enum', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json([{ code: 'seminar', label: 'Семинар' }]),
      ),
  )
  await expect(readCourseMaterialKinds()).resolves.toEqual([
    { code: 'seminar', label: 'Семинар' },
  ])
})

const legacyMaterial = {
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
  week: null,
  kind: null,
  created_at: 1,
  updated_at: 1,
}

it('maps nullable legacy metadata without rejecting the material', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json([legacyMaterial])),
  )
  await expect(readSubjectMaterials('subject-1')).resolves.toMatchObject([
    { id: 'material-1', week: null, kind: null },
  ])
})

it('sends per-file manifest before repeated file fields', async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    Response.json({
      uploaded: [legacyMaterial],
      failed: [],
      results: [{ client_id: 'a', material: legacyMaterial }],
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  const first = new File(['%PDF-one'], 'one.pdf', { type: 'application/pdf' })
  const second = new File(['%PDF-two'], 'two.pdf', { type: 'application/pdf' })
  await uploadCourseMaterials('subject-1', [
    { clientId: 'a', file: first, week: 3, kind: 'lab' },
    { clientId: 'b', file: second, week: 6, kind: 'lecture' },
  ])
  const body = fetchMock.mock.calls[0][1].body as FormData
  expect([...body.keys()]).toEqual(['items', 'file', 'file'])
  expect(JSON.parse(body.get('items') as string)).toEqual([
    { client_id: 'a', week: 3, kind: 'lab' },
    { client_id: 'b', week: 6, kind: 'lecture' },
  ])
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/exam/admin/subjects/subject-1/materials',
  )
})

it('patches only the provided metadata and preserves structured backend errors', async () => {
  const fetchMock = vi.fn().mockResolvedValue(Response.json(legacyMaterial))
  vi.stubGlobal('fetch', fetchMock)
  await updateCourseMaterialMetadata('subject-1', 'material-1', { week: 4 })
  expect(fetchMock.mock.calls[0][1].method).toBe('PATCH')
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ week: 4 })

  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json(
        {
          code: 'VALIDATION_ERROR',
          message: 'Некорректные поля запроса.',
          details: [
            { path: 'week', code: 'INVALID_FIELD', message: 'Неделя нужна.' },
          ],
        },
        { status: 422 },
      ),
    ),
  )
  await expect(
    updateCourseMaterialMetadata('subject-1', 'material-1', { week: 0 }),
  ).rejects.toMatchObject({ status: 422, code: 'VALIDATION_ERROR' })
})

it('rejects successful responses that omit the required nullable fields', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json([{ ...legacyMaterial, week: undefined }]),
      ),
  )
  await expect(readSubjectMaterials('subject-1')).rejects.toBeInstanceOf(Error)
})

it('replaces one material source with a single PDF PUT', async () => {
  const fetchMock = vi.fn().mockResolvedValue(Response.json(legacyMaterial))
  vi.stubGlobal('fetch', fetchMock)
  const pdf = new File(['%PDF-1.7 replacement'], 'replacement.pdf', {
    type: 'application/pdf',
  })
  await replaceCourseMaterialFile('subject-1', 'material-1', pdf)
  const [url, request] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/v1/exam/admin/subjects/subject-1/materials/material-1/raw')
  expect(request.method).toBe('PUT')
  expect(request.body.getAll('file')).toHaveLength(1)
  expect(request.body.get('file').name).toBe('replacement.pdf')
})

it('shows a terminal master cleaning failure through the original material status and error fields', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json([
        {
          ...legacyMaterial,
          cleaning: {
            status: 'failed',
            updated_at: 2,
            attempts: 3,
            error_message: 'Очистка материала не удалась.',
          },
        },
      ]),
    ),
  )
  await expect(readSubjectMaterials('subject-1')).resolves.toMatchObject([
    { status: 'failed', errorMessage: 'Очистка материала не удалась.' },
  ])
})
