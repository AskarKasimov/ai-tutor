import { z } from 'zod'
import { apiFetch } from '@/shared/api'
import { SubjectApiError } from '../model/subject-api-error'
import {
  isCoursePreparation,
  isPreparationConsistent,
} from '../model/preparation'
import type { Subject } from '../model/subject'
import type { AudioPreparation } from '../model/audio-preparation'

const apiBase = `${(import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)}/exam`
const id = z.string().min(1)
const subjectSchema: z.ZodType<Subject> = z.object({
  id,
  name: z.string().min(1),
  ready: z.boolean(),
  learning_ready: z.boolean(),
  exam_ready: z.boolean(),
  preparation: z
    .unknown()
    .optional()
    .refine((value) => value === undefined || isCoursePreparation(value)),
})
const adminSubjectSchema = subjectSchema.refine(isPreparationConsistent)
const audioDirectionSchema = z.object({
  version: z.string(),
  state: z.enum([
    'no_graph',
    'checking',
    'queued',
    'processing',
    'retry_wait',
    'partial',
    'ready',
    'empty',
  ]),
  total: z.number().int().nonnegative(),
  unchecked: z.number().int().nonnegative(),
  queued: z.number().int().nonnegative(),
  processing: z.number().int().nonnegative(),
  retry_wait: z.number().int().nonnegative(),
  ready: z.number().int().nonnegative(),
  failed: z.number().int().nonnegative(),
  unavailable: z.number().int().nonnegative(),
  observed_at: z.string().datetime({ offset: true }),
})
const audioPreparationSchema: z.ZodType<AudioPreparation> = z
  .object({
    subject_id: id,
    exam: audioDirectionSchema.extend({ published: z.boolean() }).nullable(),
    learn: audioDirectionSchema
      .extend({
        revision: z.number().int().nonnegative(),
      })
      .nullable(),
  })
  .superRefine((value, context) => {
    for (const direction of [value.exam, value.learn]) {
      if (!direction) continue
      const sum =
        direction.unchecked +
        direction.queued +
        direction.processing +
        direction.retry_wait +
        direction.ready +
        direction.failed +
        direction.unavailable
      if (sum !== direction.total)
        context.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'Invalid audio progress counts',
        })
    }
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

export async function listSubjects(
  signal: AbortSignal,
  requirePreparation = false,
): Promise<Subject[]> {
  const subjects = await parse(
    await request('/subjects', signal),
    z.array(subjectSchema),
    signal,
  )
  if (
    requirePreparation &&
    subjects.some((subject) => !isPreparationConsistent(subject))
  )
    throw new SubjectApiError(0, 'INVALID_RESPONSE')
  return subjects
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
    adminSubjectSchema,
    signal,
  )
}

export async function readAudioPreparation(
  subjectId: string,
  direction: 'exam' | 'learn',
  signal: AbortSignal,
): Promise<AudioPreparation> {
  const response = await request(
    `/admin/subjects/${encodeURIComponent(subjectId)}/audio-preparation?direction=${direction}`,
    signal,
  )
  const value = await parse(response, audioPreparationSchema, signal)
  if (value.subject_id !== subjectId)
    throw new SubjectApiError(0, 'INVALID_RESPONSE')
  return value
}

export type SubjectApi = {
  listSubjects: (
    signal: AbortSignal,
    requirePreparation?: boolean,
  ) => Promise<Subject[]>
  createSubject: typeof createSubject
  readAudioPreparation: typeof readAudioPreparation
}
