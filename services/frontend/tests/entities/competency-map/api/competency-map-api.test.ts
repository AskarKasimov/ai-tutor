import { afterEach, expect, it, vi } from 'vitest'
import {
  readSubjectMaterials,
  updateCourseMaterialMetadata,
  uploadCourseMaterials,
} from '@/entities/competency-map'

afterEach(() => vi.unstubAllGlobals())

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

it('sends week and kind before repeated file fields', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      Response.json({ uploaded: [legacyMaterial], failed: [] }),
    )
  vi.stubGlobal('fetch', fetchMock)
  const first = new File(['one'], 'one.pdf', { type: 'application/pdf' })
  const second = new File(['two'], 'two.pdf', { type: 'application/pdf' })
  await uploadCourseMaterials('subject-1', [first, second], 3, 'lab')
  const body = fetchMock.mock.calls[0][1].body as FormData
  expect([...body.keys()]).toEqual(['week', 'kind', 'file', 'file'])
  expect(body.get('week')).toBe('3')
  expect(body.get('kind')).toBe('lab')
  expect(fetchMock.mock.calls[0][0]).toBe(
    '/api/v1/admin/subjects/subject-1/materials',
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
