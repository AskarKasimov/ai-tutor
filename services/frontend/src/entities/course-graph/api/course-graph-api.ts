import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { i18n } from '@/shared/i18n'
import type { CourseGraph, CourseGraphBuild } from '../model/course-graph'
import { CourseGraphApiError } from '../model/course-graph-api-error'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

const snapshotSchema = z.object({
  status: z.enum(['not_requested', 'pending', 'processing', 'ready', 'failed']),
  attempts: z.number().int().nonnegative(),
  error_message: z.string(),
  stale: z.boolean(),
  inputs_ready: z.boolean(),
  updated_at: z.number().int().nonnegative(),
  graph: z
    .object({
      topics: z.array(
        z.object({
          id: z.string(),
          title: z.string(),
          question: z.string().optional(),
          provides: z.array(z.string()).optional(),
          requires: z.array(z.string()).optional(),
          parent_title: z.string().nullable().optional(),
          sources: z
            .array(
              z.object({
                material_id: z.string(),
                block_id: z.number().int(),
                start: z.number().int(),
                end: z.number().int(),
                text: z.string(),
                page: z.number().int().nullable().optional(),
                week: z.number().int().nullable().optional(),
                kind: z.string().nullable().optional(),
                filename: z.string().nullable().optional(),
                kind_label: z.string().nullable().optional(),
                material_revision: z.string().nullable().optional(),
              }),
            )
            .optional(),
        }),
      ),
      prerequisites: z.array(
        z.object({
          source: z.string(),
          target: z.string(),
          requirement: z.string().optional(),
          reason: z.string().optional(),
        }),
      ),
      hierarchy: z.array(z.object({ source: z.string(), target: z.string() })),
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
})

type Snapshot = z.infer<typeof snapshotSchema>

function invalidResponse(): CourseGraphApiError {
  return new CourseGraphApiError(
    0,
    'INVALID_RESPONSE',
    i18n.t('courseGraph.invalidResponse'),
  )
}

function parseSnapshot(value: unknown): Snapshot {
  const parsed = snapshotSchema.safeParse(value)
  if (!parsed.success) throw invalidResponse()
  return parsed.data
}

function snapshotId(subjectId: string, snapshot: Snapshot): string {
  return `${subjectId}:${snapshot.status}:${snapshot.updated_at}`
}

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
    const value: unknown = await response.json()
    signal.throwIfAborted()
    return value
  } catch {
    signal.throwIfAborted()
    throw new CourseGraphApiError(
      0,
      'INVALID_RESPONSE',
      i18n.t('courseGraph.invalidResponse'),
    )
  }
}

function toBuild(
  subjectId: string,
  snapshot: Snapshot,
): CourseGraphBuild | null {
  if (snapshot.status === 'not_requested') return null
  const statuses = {
    pending: 'queued',
    processing: 'running',
    ready: 'succeeded',
    failed: 'failed',
  } as const
  return {
    id: snapshotId(subjectId, snapshot),
    subjectId,
    status: statuses[snapshot.status],
    stage: null,
    startedAt: null,
    completedAt: null,
    error:
      snapshot.status === 'failed'
        ? { code: 'GRAPH_BUILD_FAILED', message: snapshot.error_message }
        : null,
    stale: snapshot.stale,
    inputsReady: snapshot.inputs_ready,
  }
}

export async function readLatestCourseGraphBuild(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraphBuild | null> {
  return toBuild(
    subjectId,
    parseSnapshot(
      await fetchJSON(
        `/admin/subjects/${encodeURIComponent(subjectId)}/graph`,
        signal,
      ),
    ),
  )
}

export async function startCourseGraphBuild(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraphBuild> {
  const build = toBuild(
    subjectId,
    parseSnapshot(
      await fetchJSON(
        `/admin/subjects/${encodeURIComponent(subjectId)}/graph/build`,
        signal,
        { method: 'POST' },
      ),
    ),
  )
  if (!build) throw invalidResponse()
  return build
}

export async function readCourseGraph(
  subjectId: string,
  signal: AbortSignal,
): Promise<CourseGraph> {
  const snapshot = parseSnapshot(
    await fetchJSON(
      `/admin/subjects/${encodeURIComponent(subjectId)}/graph`,
      signal,
    ),
  )
  const view = snapshot.graph
  if (!view)
    throw new CourseGraphApiError(
      404,
      'NOT_FOUND',
      i18n.t('courseGraph.requestError'),
    )
  return {
    version: snapshotId(subjectId, snapshot),
    publishedAt: snapshot.updated_at > 0 ? snapshot.updated_at : null,
    stale: snapshot.stale,
    inputsReady: snapshot.inputs_ready,
    view,
    nodes: view.topics.map((topic) => ({
      id: topic.id,
      title: topic.title,
      content: '',
      prerequisites: [
        ...new Set(
          view.prerequisites
            .filter((edge) => edge.target === topic.id)
            .map((edge) => edge.source),
        ),
      ],
      outcomes: topic.provides ?? [],
      questions: topic.question ? [topic.question] : [],
    })),
  }
}
