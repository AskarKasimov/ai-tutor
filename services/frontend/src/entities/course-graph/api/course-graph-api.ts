import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { i18n } from '@/shared/i18n'
import type { CourseGraph, CourseGraphBuild } from '../model/course-graph'
import { CourseGraphApiError } from '../model/course-graph-api-error'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

const buildSchema = z.object({
  id: z.string().min(1),
  subject_id: z.string().min(1),
  status: z.enum(['queued', 'running', 'succeeded', 'failed']),
  stage: z
    .enum(['cleaning', 'units', 'dedup', 'prerequisites', 'validation'])
    .nullable(),
  started_at: z.number().int().nonnegative().nullable(),
  completed_at: z.number().int().nonnegative().nullable(),
  error: z.object({ code: z.string(), message: z.string() }).nullable(),
})

const graphSchema = z.object({
  version: z.string().min(1),
  published_at: z.number().int().nonnegative().nullable().optional(),
  nodes: z.array(
    z.object({
      id: z.string().min(1),
      title: z.string().min(1),
      content: z.string(),
      prerequisites: z.array(z.string()),
      outcomes: z.array(z.string()),
      questions: z.array(z.string()),
    }),
  ),
})

async function responseError(response: Response): Promise<CourseGraphApiError> {
  const body: unknown = await response.json().catch(() => null)
  const parsed = z
    .object({ code: z.string().optional(), message: z.string().optional() })
    .safeParse(body)
  return new CourseGraphApiError(
    response.status,
    parsed.success ? (parsed.data.code ?? '') : '',
    parsed.success
      ? (parsed.data.message ?? i18n.t('courseGraph.requestError'))
      : i18n.t('courseGraph.requestError'),
  )
}

async function fetchJSON(
  path: string,
  signal: AbortSignal,
  init: RequestInit = {},
): Promise<unknown> {
  const response = await apiFetch(`${apiBase}${path}`, {
    ...init,
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
  })
  signal.throwIfAborted()
  if (!response.ok) throw await responseError(response)
  try {
    return await response.json()
  } catch {
    signal.throwIfAborted()
    throw new CourseGraphApiError(
      0,
      'INVALID_RESPONSE',
      i18n.t('courseGraph.invalidResponse'),
    )
  }
}

function parseBuild(value: unknown): CourseGraphBuild {
  const parsed = buildSchema.safeParse(value)
  if (!parsed.success)
    throw new CourseGraphApiError(
      0,
      'INVALID_RESPONSE',
      i18n.t('courseGraph.invalidResponse'),
    )
  return {
    id: parsed.data.id,
    subjectId: parsed.data.subject_id,
    status: parsed.data.status,
    stage: parsed.data.stage,
    startedAt: parsed.data.started_at,
    completedAt: parsed.data.completed_at,
    error: parsed.data.error,
  }
}

export async function readLatestCourseGraphBuild(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraphBuild | null> {
  try {
    return parseBuild(
      await fetchJSON(
        `/admin/subjects/${encodeURIComponent(subjectId)}/graph/build`,
        signal,
      ),
    )
  } catch (error) {
    if (error instanceof CourseGraphApiError && error.status === 404)
      return null
    throw error
  }
}

export async function startCourseGraphBuild(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraphBuild> {
  return parseBuild(
    await fetchJSON(
      `/admin/subjects/${encodeURIComponent(subjectId)}/graph/build`,
      signal,
      { method: 'POST' },
    ),
  )
}

export async function readCourseGraph(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraph> {
  const value = await fetchJSON(
    `/admin/subjects/${encodeURIComponent(subjectId)}/graph`,
    signal,
  )
  const parsed = graphSchema.safeParse(value)
  if (!parsed.success)
    throw new CourseGraphApiError(
      0,
      'INVALID_RESPONSE',
      i18n.t('courseGraph.invalidResponse'),
    )
  return {
    version: parsed.data.version,
    publishedAt: parsed.data.published_at ?? null,
    nodes: parsed.data.nodes,
  }
}
