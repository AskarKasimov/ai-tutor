import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { SubjectApiError } from '../model/subject-api-error'
import type { LearningState, Subject } from '../model/subject'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const id = z.string().min(1)
const subjectSchema: z.ZodType<Subject> = z.object({
  id,
  name: z.string().min(1),
  ready: z.boolean(),
})
const learningStateSchema: z.ZodType<LearningState> = z.object({
  subject_id: id,
  subject_name: z.string(),
  diagnostic_status: z.enum(['not_started', 'active', 'completed']),
  diagnostic_completed: z.boolean(),
  training_available: z.boolean(),
  diagnostic_session_id: id.optional(),
  active_session_id: id.optional(),
})

async function request(
  path: string,
  signal: AbortSignal,
  init: RequestInit = {},
) {
  const response = await apiFetch(`${apiBase}${path}`, {
    ...init,
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
  })
  signal.throwIfAborted()
  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null)
    const parsed = z.object({ code: z.string().optional() }).safeParse(body)
    throw new SubjectApiError(
      response.status,
      parsed.success ? parsed.data.code : '',
    )
  }
  return response
}

async function parse<T>(
  response: Response,
  schema: z.ZodType<T>,
  signal: AbortSignal,
): Promise<T> {
  let body: unknown
  try {
    body = await response.json()
  } catch {
    signal.throwIfAborted()
    throw new SubjectApiError(0, 'INVALID_RESPONSE')
  }
  const parsed = schema.safeParse(body)
  if (!parsed.success) throw new SubjectApiError(0, 'INVALID_RESPONSE')
  return parsed.data
}

export async function listSubjects(signal: AbortSignal): Promise<Subject[]> {
  return parse(
    await request('/subjects', signal),
    z.array(subjectSchema),
    signal,
  )
}

export async function createSubject(
  name: string,
  signal: AbortSignal,
): Promise<Subject> {
  return parse(
    await request('/admin/subjects', signal, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    }),
    subjectSchema,
    signal,
  )
}

export async function readLearningState(
  subjectId: string,
  signal: AbortSignal,
): Promise<LearningState> {
  const state = await parse(
    await request(
      `/subjects/${encodeURIComponent(subjectId)}/learning-state`,
      signal,
    ),
    learningStateSchema,
    signal,
  )
  if (state.subject_id !== subjectId)
    throw new SubjectApiError(0, 'INVALID_RESPONSE')
  return state
}

export type SubjectApi = {
  listSubjects: typeof listSubjects
  createSubject: typeof createSubject
  readLearningState: typeof readLearningState
}
