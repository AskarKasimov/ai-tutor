import { z } from 'zod'
import { i18n } from '@/shared/i18n'
import type {
  CourseMaterial,
  CourseMaterialKind,
} from '../model/competency-map'
import { apiFetch } from '@/shared/api'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const count = z.number().int().nonnegative()
const errorSchema = z.object({
  message: z.string(),
  details: z
    .array(
      z.object({
        path: z.string().optional(),
        message: z.string(),
        code: z.string().optional(),
      }),
    )
    .optional(),
  code: z.string().optional(),
})

import { CompetencyMapApiError } from '../model/competency-map-error'
export { CompetencyMapApiError } from '../model/competency-map-error'

async function readResponse(response: Response): Promise<unknown> {
  const data: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const parsed = errorSchema.safeParse(data)
    const key =
      response.status === 401
        ? 'unauthorized'
        : response.status === 403
          ? 'forbidden'
          : response.status === 413
            ? 'tooLarge'
            : response.status === 415
              ? 'invalidType'
              : 'requestError'
    throw new CompetencyMapApiError(
      response.status,
      parsed.success ? parsed.data.message : i18n.t(`competencyMap.${key}`),
      parsed.success ? parsed.data.details : undefined,
      parsed.success ? parsed.data.code : '',
    )
  }
  return data
}

const courseMaterialSchema = z.object({
  id: z.string(),
  subject_id: z.string(),
  filename: z.string(),
  content_type: z.string(),
  file_size: z.number().int().nonnegative(),
  s3_raw_key: z.string(),
  s3_parsed_key: z.string(),
  parsed_text: z.string(),
  char_count: z.number().int().nonnegative(),
  status: z.enum(['pending', 'parsed', 'failed']),
  week: z.number().int().positive().nullable(),
  kind: z.enum(['lecture', 'lab', 'tutorial']).nullable(),
  error_message: z.string().nullable().optional(),
  created_at: count,
  updated_at: count,
})

export async function readSubjectMaterials(
  subjectId: string,
  signal?: AbortSignal,
): Promise<CourseMaterial[]> {
  const timeout = AbortSignal.timeout(30_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials`,
    {
      credentials: 'include',
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  const parsed = z
    .array(courseMaterialSchema)
    .safeParse(await readResponse(response))
  if (!parsed.success) throw new Error(i18n.t('competencyMap.invalidResponse'))
  return parsed.data.map((item) => ({
    id: item.id,
    subjectId: item.subject_id,
    filename: item.filename,
    contentType: item.content_type,
    fileSize: item.file_size,
    s3RawKey: item.s3_raw_key,
    s3ParsedKey: item.s3_parsed_key,
    parsedText: item.parsed_text,
    charCount: item.char_count,
    status: item.status,
    week: item.week,
    kind: item.kind,
    errorMessage: item.error_message,
    createdAt: item.created_at,
    updatedAt: item.updated_at,
  }))
}

export interface CourseMaterialUploadFailedItem {
  filename: string
  code: string
  message: string
}

export interface CourseMaterialUploadItem {
  clientId: string
  file: File
  week: number
  kind: CourseMaterialKind
}
export interface CourseMaterialUploadItemResult {
  clientId: string
  material?: CourseMaterial
  error?: CourseMaterialUploadFailedItem
}
export interface CourseMaterialUploadResult {
  uploaded: CourseMaterial[]
  failed: CourseMaterialUploadFailedItem[]
  results: CourseMaterialUploadItemResult[]
}

const courseMaterialUploadFailedItemSchema = z.object({
  filename: z.string(),
  code: z.string(),
  message: z.string(),
})

const courseMaterialUploadResultSchema = z.object({
  uploaded: z.array(courseMaterialSchema),
  failed: z.array(courseMaterialUploadFailedItemSchema),
  results: z.array(
    z.object({
      client_id: z.string(),
      material: courseMaterialSchema.optional(),
      error: courseMaterialUploadFailedItemSchema.optional(),
    }),
  ),
})

export async function uploadCourseMaterials(
  subjectId: string,
  items: CourseMaterialUploadItem[],
  signal?: AbortSignal,
): Promise<CourseMaterialUploadResult> {
  const body = new FormData()
  body.append(
    'items',
    JSON.stringify(
      items.map(({ clientId, week, kind }) => ({
        client_id: clientId,
        week,
        kind,
      })),
    ),
  )
  for (const { file } of items) body.append('file', file, file.name)
  const timeout = AbortSignal.timeout(900_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials`,
    {
      method: 'POST',
      credentials: 'include',
      body,
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  const parsed = courseMaterialUploadResultSchema.safeParse(
    await readResponse(response),
  )
  if (!parsed.success) throw new Error(i18n.t('competencyMap.unknownResult'))
  return {
    uploaded: parsed.data.uploaded.map((item) => ({
      id: item.id,
      subjectId: item.subject_id,
      filename: item.filename,
      contentType: item.content_type,
      fileSize: item.file_size,
      s3RawKey: item.s3_raw_key,
      s3ParsedKey: item.s3_parsed_key,
      parsedText: item.parsed_text,
      charCount: item.char_count,
      status: item.status,
      week: item.week,
      kind: item.kind,
      errorMessage: item.error_message,
      createdAt: item.created_at,
      updatedAt: item.updated_at,
    })),
    failed: parsed.data.failed,
    results: parsed.data.results.map((item) => ({
      clientId: item.client_id,
      material: item.material
        ? {
            id: item.material.id,
            subjectId: item.material.subject_id,
            filename: item.material.filename,
            contentType: item.material.content_type,
            fileSize: item.material.file_size,
            s3RawKey: item.material.s3_raw_key,
            s3ParsedKey: item.material.s3_parsed_key,
            parsedText: item.material.parsed_text,
            charCount: item.material.char_count,
            status: item.material.status,
            week: item.material.week,
            kind: item.material.kind,
            errorMessage: item.material.error_message,
            createdAt: item.material.created_at,
            updatedAt: item.material.updated_at,
          }
        : undefined,
      error: item.error,
    })),
  }
}

export type CourseMaterialMetadataPatch = {
  week?: number
  kind?: CourseMaterialKind
}

export async function updateCourseMaterialMetadata(
  subjectId: string,
  materialId: string,
  patch: CourseMaterialMetadataPatch,
  signal?: AbortSignal,
): Promise<CourseMaterial> {
  const timeout = AbortSignal.timeout(30_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials/${encodeURIComponent(materialId)}`,
    {
      method: 'PATCH',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  const parsed = courseMaterialSchema.safeParse(await readResponse(response))
  if (!parsed.success) throw new Error(i18n.t('competencyMap.invalidResponse'))
  const item = parsed.data
  return {
    id: item.id,
    subjectId: item.subject_id,
    filename: item.filename,
    contentType: item.content_type,
    fileSize: item.file_size,
    s3RawKey: item.s3_raw_key,
    s3ParsedKey: item.s3_parsed_key,
    parsedText: item.parsed_text,
    charCount: item.char_count,
    status: item.status,
    week: item.week,
    kind: item.kind,
    errorMessage: item.error_message,
    createdAt: item.created_at,
    updatedAt: item.updated_at,
  }
}

export async function replaceCourseMaterialFile(
  subjectId: string,
  materialId: string,
  file: File,
  signal?: AbortSignal,
): Promise<CourseMaterial> {
  const body = new FormData()
  body.append('file', file, file.name)
  const timeout = AbortSignal.timeout(900_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials/${encodeURIComponent(materialId)}/raw`,
    {
      method: 'PUT',
      credentials: 'include',
      body,
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  const parsed = courseMaterialSchema.safeParse(await readResponse(response))
  if (!parsed.success) throw new Error(i18n.t('competencyMap.invalidResponse'))
  const item = parsed.data
  return {
    id: item.id,
    subjectId: item.subject_id,
    filename: item.filename,
    contentType: item.content_type,
    fileSize: item.file_size,
    s3RawKey: item.s3_raw_key,
    s3ParsedKey: item.s3_parsed_key,
    parsedText: item.parsed_text,
    charCount: item.char_count,
    status: item.status,
    week: item.week,
    kind: item.kind,
    errorMessage: item.error_message,
    createdAt: item.created_at,
    updatedAt: item.updated_at,
  }
}

export async function deleteCourseMaterial(
  subjectId: string,
  materialId: string,
  signal?: AbortSignal,
): Promise<void> {
  const timeout = AbortSignal.timeout(30_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials/${encodeURIComponent(materialId)}`,
    {
      method: 'DELETE',
      credentials: 'include',
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  await readResponse(response)
}

export async function downloadCourseMaterialRaw(
  subjectId: string,
  materialId: string,
  filename: string,
  signal?: AbortSignal,
): Promise<void> {
  const timeout = AbortSignal.timeout(60_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/materials/${encodeURIComponent(materialId)}/raw`,
    {
      credentials: 'include',
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  if (!response.ok) {
    await readResponse(response)
    return
  }
  const blob = await response.blob()
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  } finally {
    setTimeout(() => {
      URL.revokeObjectURL(url)
    }, 10_000)
  }
}
