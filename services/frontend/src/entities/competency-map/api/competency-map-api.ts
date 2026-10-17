import { z } from 'zod'
import { i18n } from '@/shared/i18n'
import type {
  CourseGraphSnapshot,
  CourseMaterial,
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
    .array(z.object({ path: z.string().optional(), message: z.string() }))
    .optional(),
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
  error_message: z.string().nullable().optional(),
  processing: z
    .object({
      stage: z.enum([
        'queued',
        'waiting_models',
        'ocr',
        'translation',
        'cleaning',
        'graph',
        'ready',
        'failed',
      ]),
      status: z.enum(['pending', 'processing', 'ready', 'failed']),
      updated_at: count,
      attempts: count,
    })
    .nullable()
    .optional(),
  cleaning: z
    .object({
      status: z.enum(['pending', 'processing', 'ready', 'failed']),
      updated_at: count,
      attempts: count,
      error_message: z.string(),
    })
    .nullable()
    .optional(),
  ocr_report: z.unknown().optional(),
  created_at: count,
  updated_at: count,
})

function getOcrWarnings(report: unknown): string[] {
  if (!report || typeof report !== 'object' || Array.isArray(report)) return []
  const record = report as Record<string, unknown>
  const warnings = [record.warnings, record.pipeline_warnings].flatMap(
    (value) =>
      Array.isArray(value)
        ? value.filter(
            (warning): warning is string => typeof warning === 'string',
          )
        : [],
  )
  return warnings.filter(
    (warning, index) => warnings.indexOf(warning) === index,
  )
}

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
    errorMessage: item.error_message,
    processing: item.processing
      ? {
          stage: item.processing.stage,
          status: item.processing.status,
          updatedAt: item.processing.updated_at,
          attempts: item.processing.attempts,
        }
      : undefined,
    cleaning: item.cleaning
      ? {
          status: item.cleaning.status,
          updatedAt: item.cleaning.updated_at,
          attempts: item.cleaning.attempts,
          errorMessage: item.cleaning.error_message,
        }
      : undefined,
    ocrWarnings: getOcrWarnings(item.ocr_report),
    createdAt: item.created_at,
    updatedAt: item.updated_at,
  }))
}

export interface CourseMaterialUploadFailedItem {
  filename: string
  code: string
  message: string
}

export interface CourseMaterialUploadResult {
  uploaded: CourseMaterial[]
  failed: CourseMaterialUploadFailedItem[]
}

const courseMaterialUploadFailedItemSchema = z.object({
  filename: z.string(),
  code: z.string(),
  message: z.string(),
})

const courseMaterialUploadResultSchema = z.object({
  uploaded: z.array(courseMaterialSchema),
  failed: z.array(courseMaterialUploadFailedItemSchema),
})

export async function uploadCourseMaterials(
  subjectId: string,
  files: File[],
  signal?: AbortSignal,
): Promise<CourseMaterialUploadResult> {
  const body = new FormData()
  for (const file of files) {
    body.append('file', file, file.name)
  }
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
      errorMessage: item.error_message,
      processing: item.processing
        ? {
            stage: item.processing.stage,
            status: item.processing.status,
            updatedAt: item.processing.updated_at,
            attempts: item.processing.attempts,
          }
        : undefined,
      cleaning: item.cleaning
        ? {
            status: item.cleaning.status,
            updatedAt: item.cleaning.updated_at,
            attempts: item.cleaning.attempts,
            errorMessage: item.cleaning.error_message,
          }
        : undefined,
      ocrWarnings: getOcrWarnings(item.ocr_report),
      createdAt: item.created_at,
      updatedAt: item.updated_at,
    })),
    failed: parsed.data.failed,
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

const courseGraphSourceSchema = z.object({
  material_id: z.string(),
  page: z.number().int().nullable().optional(),
  week: z.number().int().nullable().optional(),
})

const courseGraphTopicSchema = z.object({
  id: z.string(),
  title: z.string(),
  question: z.string().optional(),
  text: z.string().optional(),
  provides: z.array(z.string()).optional(),
  requires: z.array(z.string()).optional(),
  sources: z.array(courseGraphSourceSchema).optional(),
  parent_title: z.string().nullable().optional(),
})

const courseGraphPrerequisiteSchema = z.object({
  source: z.string(),
  target: z.string(),
  requirement: z.string().optional(),
  reason: z.string().optional(),
})

const courseGraphHierarchySchema = z.object({
  source: z.string(),
  target: z.string(),
})

const courseGraphSnapshotSchema = z.object({
  status: z.enum(['not_requested', 'pending', 'processing', 'ready', 'failed']),
  attempts: z.number().int().nonnegative(),
  error_message: z.string(),
  graph: z
    .object({
      topics: z.array(courseGraphTopicSchema),
      prerequisites: z.array(courseGraphPrerequisiteSchema),
      hierarchy: z.array(courseGraphHierarchySchema),
      quality: z
        .object({
          acyclic: z.boolean(),
          topics: z.number().int().optional(),
          requirements: z.number().int().optional(),
          unresolved_requirements: z.number().int().optional(),
        })
        .passthrough()
        .optional(),
    })
    .passthrough()
    .nullable(),
  graph_source: z.enum(['built', 'fixture', 'none']),
  stale: z.boolean(),
  inputs_ready: z.boolean(),
  updated_at: z.number().int().nonnegative(),
})

function toSnapshot(raw: unknown): CourseGraphSnapshot {
  const parsed = courseGraphSnapshotSchema.safeParse(raw)
  if (!parsed.success) throw new Error(i18n.t('competencyMap.invalidResponse'))
  const data = parsed.data
  return {
    status: data.status,
    attempts: data.attempts,
    errorMessage: data.error_message,
    graph: data.graph,
    graphSource: data.graph_source,
    stale: data.stale,
    inputsReady: data.inputs_ready,
    updatedAt: data.updated_at,
  }
}

export async function readCourseGraph(
  subjectId: string,
  signal?: AbortSignal,
): Promise<CourseGraphSnapshot> {
  const timeout = AbortSignal.timeout(30_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/graph`,
    {
      credentials: 'include',
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  return toSnapshot(await readResponse(response))
}

// The admin's "build graph" button: starts a build of the current materials.
export async function requestCourseGraphBuild(
  subjectId: string,
  signal?: AbortSignal,
): Promise<CourseGraphSnapshot> {
  const timeout = AbortSignal.timeout(30_000)
  const response = await apiFetch(
    `${apiBase}/admin/subjects/${encodeURIComponent(subjectId)}/graph/build`,
    {
      method: 'POST',
      credentials: 'include',
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    },
  )
  return toSnapshot(await readResponse(response))
}
