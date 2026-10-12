import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { LearningPlanApiError } from '../model/learning-plan-api-error'
import type { LearningMessage, LearningPlan } from '../model/learning-plan'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const id = z.string().min(1)
const count = z.number().int().nonnegative()
const itemSchema = z.object({
  index: count,
  competency_id: id,
  competency_name: z.string(),
  outcome_id: id,
  outcome_name: z.string(),
  theory_minutes: count,
  practice_minutes: count,
  in_plan: z.boolean(),
  status: z.enum(['pending', 'current', 'passed', 'review']),
  attempts: count,
})
const planSchema: z.ZodType<LearningPlan> = z.object({
  id,
  diagnostic_session_id: id,
  subject_id: id,
  subject_name: z.string(),
  hours: count.min(1),
  detail: z.enum(['brief', 'standard', 'deep']),
  status: z.enum(['active', 'completed']),
  current_index: count.nullable(),
  progress_percent: count.max(100),
  planned_minutes: count,
  pending_answer: z.string().nullable(),
  items: z.array(itemSchema),
})
const messagesSchema: z.ZodType<{ items: LearningMessage[] }> = z.object({
  items: z.array(
    z.object({
      sequence: count.min(1),
      item_index: count,
      role: z.enum(['assistant', 'user']),
      kind: z.enum(['theory', 'practice', 'answer', 'feedback']),
      text: z.string(),
      created_at: count,
    }),
  ),
})

// The teacher answers in tens of seconds; speech recognition adds more.
const modelTimeout = 180_000

async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  signal: AbortSignal,
  init: RequestInit = {},
  timeoutMs = 30_000,
): Promise<T> {
  const response = await apiFetch(`${apiBase}${path}`, {
    ...init,
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)]),
  })
  signal.throwIfAborted()
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const parsed = z.object({ code: z.string().optional() }).safeParse(body)
    throw new LearningPlanApiError(
      response.status,
      parsed.success ? parsed.data.code : '',
    )
  }
  const parsed = schema.safeParse(body)
  if (!parsed.success) throw new LearningPlanApiError(502, 'INVALID_RESPONSE')
  return parsed.data
}

const plan = (id: string) => `/learning-plans/${encodeURIComponent(id)}`

// The plan of a diagnostic, or null before the student chose their hours.
export async function findLearningPlan(
  diagnosticId: string,
  signal: AbortSignal,
): Promise<LearningPlan | null> {
  try {
    return await request(
      `/diagnostic-sessions/${encodeURIComponent(diagnosticId)}/learning-plan`,
      planSchema,
      signal,
    )
  } catch (error) {
    if (error instanceof LearningPlanApiError && error.status === 404)
      return null
    throw error
  }
}

export function createLearningPlan(
  diagnosticId: string,
  hours: number,
  signal: AbortSignal,
) {
  return request(
    `/diagnostic-sessions/${encodeURIComponent(diagnosticId)}/learning-plan`,
    planSchema,
    signal,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ hours }),
    },
  )
}

export function readLearningPlan(planId: string, signal: AbortSignal) {
  return request(plan(planId), planSchema, signal)
}

export async function readLearningMessages(
  planId: string,
  index: number,
  signal: AbortSignal,
) {
  const page = await request(
    `${plan(planId)}/items/${index}/messages`,
    messagesSchema,
    signal,
  )
  return page.items
}

// Asks the teacher for the theory and the first practice question of the
// current topic; repeated calls do not rewrite an opened lesson.
export function openLesson(planId: string, signal: AbortSignal) {
  return request(
    `${plan(planId)}/open`,
    planSchema,
    signal,
    { method: 'POST' },
    modelTimeout,
  )
}

export function answerLesson(planId: string, audio: Blob, signal: AbortSignal) {
  const extension = audio.type.includes('ogg')
    ? 'ogg'
    : audio.type.includes('wav')
      ? 'wav'
      : 'webm'
  const form = new FormData()
  form.append('audio', audio, `answer.${extension}`)
  return request(
    `${plan(planId)}/answers`,
    planSchema,
    signal,
    { method: 'POST', body: form },
    modelTimeout,
  )
}

export function retryLessonAnswer(planId: string, signal: AbortSignal) {
  return request(
    `${plan(planId)}/answers/retry`,
    planSchema,
    signal,
    { method: 'POST' },
    modelTimeout,
  )
}

export type LearningPlanApi = {
  findLearningPlan: typeof findLearningPlan
  createLearningPlan: typeof createLearningPlan
  readLearningPlan: typeof readLearningPlan
  readLearningMessages: typeof readLearningMessages
  openLesson: typeof openLesson
  answerLesson: typeof answerLesson
  retryLessonAnswer: typeof retryLessonAnswer
}
