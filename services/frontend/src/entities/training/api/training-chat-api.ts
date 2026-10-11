import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { TrainingApiError } from '../model/training-api-error'
import type {
  TrainingChatCheck,
  TrainingChatMessage,
  TrainingChatPage,
  TrainingChatPendingTurn,
  TrainingChatTurnResult,
} from '../model/training-chat'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const id = z.string().min(1)
const count = z.number().int().nonnegative()
const source = z.enum(['text', 'voice'])
const checkSchema: z.ZodType<TrainingChatCheck> = z
  .object({
    revision: count,
    final_answer: id,
    source,
    transcription_id: id.nullable().optional(),
    score: z.union([z.literal(0), z.literal(1), z.literal(2)]),
    max_score: z.literal(2),
    verdict: z.enum(['incorrect', 'partial', 'correct']),
  })
  .strict()
  .refine(
    (check) =>
      check.verdict === ['incorrect', 'partial', 'correct'][check.score],
  )
const messageSchema: z.ZodType<TrainingChatMessage> = z
  .object({
    id,
    sequence: count.min(1),
    role: z.enum(['user', 'assistant']),
    kind: z.enum(['welcome', 'message', 'check']),
    text: id,
    source: source.optional(),
    created_at: count,
  })
  .strict()
const pendingSchema: z.ZodType<TrainingChatPendingTurn> = z
  .object({
    id,
    kind: z.enum(['message', 'check']),
    source,
    status: z.enum(['processing', 'failed']),
    user_text: z.string().nullable(),
    retry_after_ms: count,
  })
  .strict()
const pageSchema: z.ZodType<TrainingChatPage> = z
  .object({
    session_id: id,
    exercise_id: id,
    revision: count,
    status: z.enum(['active', 'closed']),
    items: z.array(messageSchema),
    next_cursor: id.nullable().optional(),
    latest_check: checkSchema.nullable(),
    pending_turn: pendingSchema.nullable(),
  })
  .strict()
  .refine((page) => {
    const sequences = page.items.map((item) => item.sequence)
    return (
      sequences.every(
        (value, index) => index === 0 || sequences[index - 1]! < value,
      ) &&
      (!page.latest_check || page.latest_check.revision <= page.revision)
    )
  })
const turnSchema: z.ZodType<TrainingChatTurnResult> = z
  .object({
    session_id: id,
    exercise_id: id,
    revision: count.min(1),
    turn_id: id,
    messages: z.tuple([messageSchema, messageSchema]),
    latest_check: checkSchema.nullable(),
  })
  .strict()
  .refine((turn) => {
    const [user, assistant] = turn.messages
    return (
      user.role === 'user' &&
      assistant.role === 'assistant' &&
      user.sequence < assistant.sequence &&
      user.kind === assistant.kind &&
      (!turn.latest_check || turn.latest_check.revision <= turn.revision) &&
      (user.kind !== 'check' || turn.latest_check?.revision === turn.revision)
    )
  })

async function request(
  path: string,
  signal: AbortSignal,
  init: RequestInit = {},
  timeoutMs = 240_000,
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

const base = (sessionId: string, exerciseId: string) =>
  `/training-sessions/${encodeURIComponent(sessionId)}/exercises/${encodeURIComponent(exerciseId)}/chat`

export async function readTrainingChat(
  sessionId: string,
  exerciseId: string,
  cursor: string | undefined,
  limit: number,
  signal: AbortSignal,
) {
  const query = new URLSearchParams({ limit: String(limit) })
  if (cursor) query.set('cursor', cursor)
  const page = await parse(
    await request(
      `${base(sessionId, exerciseId)}?${query}`,
      signal,
      {},
      30_000,
    ),
    pageSchema,
    signal,
  )
  if (page.session_id !== sessionId || page.exercise_id !== exerciseId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return page
}

async function send(
  sessionId: string,
  exerciseId: string,
  key: string,
  body: BodyInit,
  contentType: string | undefined,
  signal: AbortSignal,
) {
  const headers: Record<string, string> = { 'Idempotency-Key': key }
  if (contentType) headers['Content-Type'] = contentType
  const result = await parse(
    await request(`${base(sessionId, exerciseId)}/turns`, signal, {
      method: 'POST',
      headers,
      body,
    }),
    turnSchema,
    signal,
  )
  if (result.session_id !== sessionId || result.exercise_id !== exerciseId)
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return result
}

export function createTrainingChatText(
  key: string,
  revision: number,
  kind: 'message' | 'check',
  text: string,
) {
  return { key, body: JSON.stringify({ revision, kind, text }) }
}

export function createTrainingChatVoice(
  key: string,
  revision: number,
  kind: 'message' | 'check',
  audio: Blob,
) {
  const body = new FormData()
  body.append('revision', String(revision))
  body.append('kind', kind)
  const extension = audio.type.includes('ogg')
    ? 'ogg'
    : audio.type.includes('wav')
      ? 'wav'
      : 'webm'
  body.append('audio', audio, `message.${extension}`)
  return { key, body }
}

export function sendTrainingChatText(
  sessionId: string,
  exerciseId: string,
  input: ReturnType<typeof createTrainingChatText>,
  signal: AbortSignal,
) {
  return send(
    sessionId,
    exerciseId,
    input.key,
    input.body,
    'application/json',
    signal,
  )
}
export function sendTrainingChatVoice(
  sessionId: string,
  exerciseId: string,
  input: ReturnType<typeof createTrainingChatVoice>,
  signal: AbortSignal,
) {
  return send(sessionId, exerciseId, input.key, input.body, undefined, signal)
}

export async function retryTrainingChatTurn(
  sessionId: string,
  exerciseId: string,
  turnId: string,
  signal: AbortSignal,
) {
  const result = await parse(
    await request(
      `${base(sessionId, exerciseId)}/turns/${encodeURIComponent(turnId)}/retry`,
      signal,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      },
    ),
    turnSchema,
    signal,
  )
  if (
    result.session_id !== sessionId ||
    result.exercise_id !== exerciseId ||
    result.turn_id !== turnId
  )
    throw new TrainingApiError(0, 'INVALID_RESPONSE')
  return result
}

export async function cancelTrainingChatTurn(
  sessionId: string,
  exerciseId: string,
  turnId: string,
  signal: AbortSignal,
) {
  const response = await request(
    `${base(sessionId, exerciseId)}/turns/${encodeURIComponent(turnId)}/cancel`,
    signal,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    },
  )
  if (response.status !== 204) throw new TrainingApiError(0, 'INVALID_RESPONSE')
}

export type TrainingChatApi = {
  readTrainingChat: typeof readTrainingChat
  createTrainingChatText: typeof createTrainingChatText
  createTrainingChatVoice: typeof createTrainingChatVoice
  sendTrainingChatText: typeof sendTrainingChatText
  sendTrainingChatVoice: typeof sendTrainingChatVoice
  retryTrainingChatTurn: typeof retryTrainingChatTurn
  cancelTrainingChatTurn: typeof cancelTrainingChatTurn
}
