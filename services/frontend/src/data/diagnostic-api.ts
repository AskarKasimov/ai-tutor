import { z } from 'zod'
import { apiFetch } from './api-fetch'
import type {
  DiagnosticProgress,
  DiagnosticResult,
  DiagnosticTask,
} from '../shared/domain'

export const diagnosticApiBase = (
  import.meta.env.VITE_API_BASE_URL || '/api/v1'
).replace(/\/$/, '')
const id = z.string().min(1)
const count = z.number().int().nonnegative()
const score = count.max(2)
const verdict = z.enum(['correct', 'partial', 'incorrect'])
const criteria = z.array(
  z.object({ key: id, satisfied: z.boolean(), explanation: id }),
)
const feedback = z.array(id).length(3)
const task = z.object({
  variant_task_id: id,
  source_task_id: id,
  role: z.enum(['main', 'basic']),
  competency_id: id,
  competency_name: z.string(),
  constituent_id: id,
  constituent_name: z.string(),
  outcome_id: id,
  outcome_name: z.string(),
  question: id,
  options: z.array(z.string()),
  voice_instruction: id,
})
const progressSchema: z.ZodType<DiagnosticProgress> = z
  .object({
    session_id: id,
    status: z.enum(['active', 'completed']),
    completed_tasks: count,
    skipped_tasks: count,
    total_tasks: count.min(1),
    current: task.optional(),
    score: score.optional(),
    grader_score: score.optional(),
    grader_max_score: count.min(1).max(2).optional(),
    verdict: verdict.optional(),
    criterion_results: criteria.optional(),
    feedback: feedback.optional(),
  })
  .refine(
    (p) =>
      p.completed_tasks + p.skipped_tasks <= p.total_tasks &&
      (p.status === 'active'
        ? !!p.current && p.completed_tasks + p.skipped_tasks < p.total_tasks
        : !p.current && p.completed_tasks + p.skipped_tasks === p.total_tasks),
  )
const answerSchema = z
  .object({
    variant_task_id: id,
    source_task_id: id,
    competency_id: id,
    outcome_id: id,
    role: z.enum(['main', 'basic']),
    task,
    transcription_id: id,
    text: id,
    grader_score: score,
    grader_max_score: count.min(1).max(2),
    score,
    verdict,
    criterion_results: criteria,
    feedback,
    created_at: count,
  })
  .refine(
    (a) =>
      a.score <= (a.role === 'main' ? 2 : 1) &&
      a.grader_max_score === (a.role === 'main' ? 2 : 1) &&
      a.task.variant_task_id === a.variant_task_id &&
      a.task.role === a.role,
  )
const resultSchema: z.ZodType<DiagnosticResult> = z
  .object({
    session_id: id,
    status: z.literal('completed'),
    variant_id: id,
    map_revision: count.min(1),
    included_competency_count: count.min(1),
    skipped_competencies: z.array(
      z.object({ competency_id: id, competency_name: z.string(), code: id }),
    ),
    completed_tasks: count,
    total_tasks: count.min(1),
    diagnostic_score: count,
    maximum_score: count.min(1),
    answers: z.array(answerSchema),
    untested_basics: z.array(task),
  })
  .refine(
    (r) =>
      r.diagnostic_score <= r.maximum_score &&
      r.completed_tasks === r.answers.length &&
      r.completed_tasks + r.untested_basics.length === r.total_tasks,
  )

export class DiagnosticApiError extends Error {
  constructor(
    readonly status: number,
    readonly code = '',
  ) {
    super(`Diagnostic request failed: ${status}`)
    this.name = 'DiagnosticApiError'
  }
}
async function request(
  path: string,
  signal: AbortSignal,
  init: RequestInit = {},
  timeoutMs = 120_000,
) {
  const response = await apiFetch(`${diagnosticApiBase}${path}`, {
    ...init,
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)]),
  })
  signal.throwIfAborted()
  if (!response.ok) {
    const error: unknown = await response.json().catch(() => null)
    const parsed = z.object({ code: z.string().optional() }).safeParse(error)
    throw new DiagnosticApiError(
      response.status,
      parsed.success ? parsed.data.code : '',
    )
  }
  return response
}
async function parse<T>(response: Response, schema: z.ZodType<T>): Promise<T> {
  const value = schema.safeParse(await response.json())
  if (!value.success) throw new DiagnosticApiError(0, 'INVALID_RESPONSE')
  return value.data
}
export async function createVariant(key: string, signal: AbortSignal) {
  return parse(
    await request('/variants', signal, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
    }),
    z.object({ id }),
  )
}
export async function startDiagnostic(
  variantId: string,
  key: string,
  signal: AbortSignal,
) {
  return parse(
    await request('/diagnostic-sessions', signal, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
      body: JSON.stringify({ variant_id: variantId }),
    }),
    progressSchema,
  )
}
export async function readDiagnostic(sessionId: string, signal: AbortSignal) {
  return parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(sessionId)}`,
      signal,
    ),
    progressSchema,
  )
}
export type DiagnosticSubmission = {
  sessionId: string
  taskId: string
  role: DiagnosticTask['role']
  key: string
  body: FormData
}
export function createSubmission(
  sessionId: string,
  taskId: string,
  blob: Blob,
  role: DiagnosticTask['role'],
): DiagnosticSubmission {
  const body = new FormData()
  const extension = blob.type.includes('ogg')
    ? 'ogg'
    : blob.type.includes('wav')
      ? 'wav'
      : 'webm'
  body.append('audio', blob, `answer.${extension}`)
  body.append('variant_task_id', taskId)
  return { sessionId, taskId, role, key: crypto.randomUUID(), body }
}
export async function submitDiagnostic(
  input: DiagnosticSubmission,
  signal: AbortSignal,
) {
  const progress = await parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(input.sessionId)}/answers`,
      signal,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': input.key },
        body: input.body,
      },
      240_000,
    ),
    progressSchema,
  )
  if (
    progress.session_id !== input.sessionId ||
    progress.score === undefined ||
    progress.grader_max_score !== (input.role === 'main' ? 2 : 1) ||
    progress.score > progress.grader_max_score ||
    !progress.feedback ||
    !progress.verdict
  )
    throw new DiagnosticApiError(0, 'INVALID_RESPONSE')
  return progress
}
export async function diagnosticAudio(sessionId: string, signal: AbortSignal) {
  const response = await request(
    `/diagnostic-sessions/${encodeURIComponent(sessionId)}/current/audio`,
    signal,
  )
  const blob = await response.blob()
  if (
    !blob.size ||
    !response.headers.get('Content-Type')?.includes('audio/wav')
  )
    throw new DiagnosticApiError(0, 'INVALID_RESPONSE')
  return blob
}
export async function readDiagnosticResult(
  sessionId: string,
  signal: AbortSignal,
) {
  const result = await parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(sessionId)}/result`,
      signal,
    ),
    resultSchema,
  )
  if (result.session_id !== sessionId)
    throw new DiagnosticApiError(0, 'INVALID_RESPONSE')
  return result
}
