import { z } from 'zod'
import { apiFetch, fetchStoredAudio } from '@/shared/api'
import { TrainingApiError } from '../model/training-api-error'
import type {
  TrainingAttempt,
  TrainingAudioMetadata,
  TrainingHistory,
  TrainingPreview,
  TrainingProgress,
  TrainingSubmission,
  TrainingTarget,
} from '../model/training'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const id = z.string().min(1)
const count = z.number().int().nonnegative()
const score = count.max(2)
const targetSchema: z.ZodType<TrainingTarget> = z.object({
  kind: id,
  label: id,
  competency_id: id,
  competency_name: z.string(),
  outcome_id: id,
  outcome_name: z.string(),
  original_score: score.nullable(),
  original_max_score: count.max(2).nullable(),
  last_score: score.nullable(),
})
const exerciseSchema = z.object({
  exercise_id: id,
  outcome_id: id,
  outcome_name: z.string(),
  question: id,
  options: z.array(z.string()),
  voice_instruction: id,
})
const criterionSchema = z.object({
  key: id,
  satisfied: z.boolean(),
  explanation: id,
})
const attemptSchema: z.ZodType<TrainingAttempt> = z
  .object({
    sequence: count.min(1),
    exercise_id: id,
    round: count.min(1),
    target_index: count,
    transcription_id: id,
    text: id,
    score,
    max_score: z.literal(2),
    verdict: z.enum(['correct', 'partial', 'incorrect']),
    criterion_results: z.array(criterionSchema),
    feedback: z.array(id).length(3),
    created_at: count,
  })
  .refine((attempt) => attempt.score <= attempt.max_score)
const progressSchema: z.ZodType<TrainingProgress> = z
  .object({
    session_id: id,
    diagnostic_session_id: id,
    subject_id: id,
    subject_name: z.string(),
    mode: z.enum(['focused', 'free_practice']),
    status: z.literal('active'),
    round: count.min(1),
    answer_count: count,
    targets: z.array(targetSchema),
    current: exerciseSchema,
    answer: attemptSchema.optional(),
  })
  .refine(
    (progress) =>
      !progress.answer || progress.answer.sequence <= progress.answer_count,
  )
const previewSchema: z.ZodType<TrainingPreview> = z
  .object({
    diagnostic_session_id: id,
    subject_id: id,
    subject_name: z.string(),
    mode: z.enum(['focused', 'free_practice']),
    plan_revision: count.min(1),
    diagnostic_score: count,
    maximum_score: count.min(1),
    status: z.enum(['ready', 'no_practice_tasks']),
    confirmed_gaps: z.array(targetSchema),
    partial_competencies: z.array(targetSchema),
    topics: z.array(targetSchema),
  })
  .refine((preview) => preview.diagnostic_score <= preview.maximum_score)
const audioSchema: z.ZodType<TrainingAudioMetadata> = z
  .object({
    exercise_id: id,
    status: z.enum([
      'missing',
      'pending',
      'processing',
      'ready',
      'failed',
      'cancelled',
    ]),
    audio_url: z.string().nullable(),
  })
  .refine((audio) =>
    audio.status === 'ready'
      ? !!audio.audio_url &&
        /^\/task-audio\/[A-Za-z0-9_-]{1,256}\/file$/.test(audio.audio_url)
      : audio.audio_url === null,
  )
const historySchema: z.ZodType<TrainingHistory> = z.object({
  items: z.array(attemptSchema),
  targets: z.array(targetSchema),
  next_cursor: id.optional(),
})

async function request(
  path: string,
  signal: AbortSignal,
  init: RequestInit = {},
  timeoutMs = 30_000,
) {
  const response = await apiFetch(`${apiBase}${path}`, {
    ...init,
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)]),
  })
  signal.throwIfAborted()
  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null)
    const parsed = z.object({ code: z.string().optional() }).safeParse(body)
    throw new TrainingApiError(
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
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  }
  const parsed = schema.safeParse(body)
  if (!parsed.success) throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return parsed.data
}

export async function readTrainingPreview(
  diagnosticId: string,
  signal: AbortSignal,
) {
  const preview = await parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(diagnosticId)}/training/preview`,
      signal,
    ),
    previewSchema,
    signal,
  )
  if (preview.diagnostic_session_id !== diagnosticId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return preview
}

export async function startTraining(
  diagnosticId: string,
  key: string,
  planRevision: number | undefined,
  signal: AbortSignal,
) {
  const progress = await parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(diagnosticId)}/training`,
      signal,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
        body: JSON.stringify(
          planRevision === undefined ? {} : { plan_revision: planRevision },
        ),
      },
      120_000,
    ),
    progressSchema,
    signal,
  )
  if (
    progress.diagnostic_session_id !== diagnosticId ||
    (planRevision === undefined && progress.mode !== 'focused') ||
    (planRevision !== undefined && progress.mode !== 'free_practice')
  )
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return progress
}

export async function findTrainingForDiagnostic(
  diagnosticId: string,
  signal: AbortSignal,
) {
  const progress = await parse(
    await request(
      `/diagnostic-sessions/${encodeURIComponent(diagnosticId)}/training`,
      signal,
    ),
    progressSchema,
    signal,
  )
  if (progress.diagnostic_session_id !== diagnosticId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return progress
}

export async function readTrainingSession(
  sessionId: string,
  signal: AbortSignal,
) {
  const progress = await parse(
    await request(
      `/training-sessions/${encodeURIComponent(sessionId)}`,
      signal,
    ),
    progressSchema,
    signal,
  )
  if (progress.session_id !== sessionId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return progress
}

export function createTrainingSubmission(
  sessionId: string,
  exerciseId: string,
  blob: Blob,
): TrainingSubmission {
  const body = new FormData()
  const extension = blob.type.includes('ogg')
    ? 'ogg'
    : blob.type.includes('wav')
      ? 'wav'
      : 'webm'
  body.append('audio', blob, `answer.${extension}`)
  body.append('exercise_id', exerciseId)
  return { sessionId, exerciseId, key: crypto.randomUUID(), body }
}

export async function submitTraining(
  input: TrainingSubmission,
  signal: AbortSignal,
) {
  const progress = await parse(
    await request(
      `/training-sessions/${encodeURIComponent(input.sessionId)}/answers`,
      signal,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': input.key },
        body: input.body,
      },
      240_000,
    ),
    progressSchema,
    signal,
  )
  if (
    progress.session_id !== input.sessionId ||
    !progress.answer ||
    progress.answer.exercise_id !== input.exerciseId ||
    progress.answer.max_score !== 2 ||
    progress.answer.score > 2 ||
    !progress.answer.verdict ||
    progress.answer.feedback.length !== 3
  )
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return progress
}

export async function readTrainingHistory(
  sessionId: string,
  cursor: string | undefined,
  limit: number,
  signal: AbortSignal,
) {
  const query = new URLSearchParams({ limit: String(limit) })
  if (cursor !== undefined) query.set('cursor', cursor)
  return parse(
    await request(
      `/training-sessions/${encodeURIComponent(sessionId)}/history?${query}`,
      signal,
    ),
    historySchema,
    signal,
  )
}

export async function readTrainingAudio(
  sessionId: string,
  exerciseId: string,
  signal: AbortSignal,
) {
  const audio = await parse(
    await request(
      `/training-sessions/${encodeURIComponent(sessionId)}/current/audio?exercise_id=${encodeURIComponent(exerciseId)}`,
      signal,
    ),
    audioSchema,
    signal,
  )
  if (audio.exercise_id !== exerciseId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return audio
}

export async function fetchTrainingAudioFile(
  audioUrl: string,
  signal: AbortSignal,
): Promise<Blob> {
  return fetchStoredAudio(audioUrl, signal)
}

export async function regenerateTrainingAudio(
  sessionId: string,
  exerciseId: string,
  signal: AbortSignal,
) {
  const audio = await parse(
    await request(
      `/training-sessions/${encodeURIComponent(sessionId)}/current/audio/regenerate?exercise_id=${encodeURIComponent(exerciseId)}`,
      signal,
      { method: 'POST' },
    ),
    audioSchema,
    signal,
  )
  if (audio.exercise_id !== exerciseId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return audio
}

export type TrainingApi = {
  readTrainingPreview: typeof readTrainingPreview
  startTraining: typeof startTraining
  findTrainingForDiagnostic: typeof findTrainingForDiagnostic
  readTrainingSession: typeof readTrainingSession
  createTrainingSubmission: typeof createTrainingSubmission
  submitTraining: typeof submitTraining
  readTrainingHistory: typeof readTrainingHistory
  readTrainingAudio: typeof readTrainingAudio
  fetchTrainingAudioFile: typeof fetchTrainingAudioFile
  regenerateTrainingAudio: typeof regenerateTrainingAudio
}
