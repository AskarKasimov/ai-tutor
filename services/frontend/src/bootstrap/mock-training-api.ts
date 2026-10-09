import { createDemoAudio } from '@/shared/lib'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'
import type {
  TrainingAttempt,
  TrainingExercise,
  TrainingPreview,
  TrainingProgress,
  TrainingTarget,
} from '@/entities/training'

const subjectId = 'subject:intro-to-ml'
const subjectName = 'Введение в ML'
const targets: TrainingTarget[] = [
  {
    kind: 'confirmed_gap',
    label: 'Неосвоенные темы',
    competency_id: 'demo-competency',
    competency_name: 'Виды задач машинного обучения',
    outcome_id: 'demo-outcome',
    outcome_name: 'Различать классификацию и регрессию',
    original_score: 0,
    original_max_score: 2,
    last_score: null,
  },
  {
    kind: 'partial_competency',
    label: 'Частичные пробелы в знаниях',
    competency_id: 'demo-competency',
    competency_name: 'Виды задач машинного обучения',
    outcome_id: 'demo-outcome',
    outcome_name: 'Объяснять выбор модели машинного обучения',
    original_score: 1,
    original_max_score: 2,
    last_score: null,
  },
]
const exerciseFor = (
  round: number,
  target: TrainingTarget,
): TrainingExercise => ({
  exercise_id: `demo-exercise-${crypto.randomUUID()}`,
  outcome_id: target.outcome_id,
  outcome_name: target.outcome_name,
  question:
    round % 2
      ? 'Как определить, что задача относится к классификации?'
      : 'Какой тип задачи предсказывает числовое значение?',
  options: ['Классификация', 'Регрессия'],
  voice_instruction: 'Назовите тип задачи и объясните свой выбор.',
})
type TrainingSession = {
  userId: string
  diagnosticId: string
  id: string
  mode: 'focused' | 'free_practice'
  targets: TrainingTarget[]
  answers: TrainingAttempt[]
  exercise: TrainingExercise
  accepted: Map<
    string,
    {
      result: TrainingProgress
      exerciseId: string
      audioBytes: Uint8Array
      audioType: string
      audioName: string
    }
  >
}
const sessions = new Map<string, TrainingSession>()
const sessionByDiagnostic = new Map<string, string>()
const starts = new Map<string, string>()

export function resetMockTraining() {
  sessions.clear()
  sessionByDiagnostic.clear()
  starts.clear()
}
export function mockTrainingSessionForDiagnostic(
  userId: string,
  diagnosticId: string,
) {
  const id = sessionByDiagnostic.get(`${userId}:${diagnosticId}`)
  return id && sessions.get(id)?.userId === userId ? id : undefined
}

function json(body: unknown, status = 200) {
  return Response.json(body, { status })
}
function failed(code: string, status: number) {
  return json({ code }, status)
}
function trainingTarget(
  target: TrainingTarget,
  score?: number,
): TrainingTarget {
  return { ...target, last_score: score ?? target.last_score }
}
function current(session: TrainingSession): TrainingProgress {
  return {
    session_id: session.id,
    diagnostic_session_id: session.diagnosticId,
    subject_id: subjectId,
    subject_name: subjectName,
    mode: session.mode,
    status: 'active',
    round: session.answers.length + 1,
    answer_count: session.answers.length,
    targets: session.targets.map((target) => {
      const latest = [...session.answers]
        .reverse()
        .find((a) => a.target_index === session.targets.indexOf(target))
      return trainingTarget(target, latest?.score)
    }),
    current: session.exercise,
    ...(session.answers.length ? { answer: session.answers.at(-1) } : {}),
  }
}
function preview(
  diagnosticId: string,
  mode: 'focused' | 'free_practice',
): TrainingPreview {
  return {
    diagnostic_session_id: diagnosticId,
    subject_id: subjectId,
    subject_name: subjectName,
    mode,
    plan_revision: 1,
    diagnostic_score: 2,
    maximum_score: 2,
    status: 'ready',
    confirmed_gaps: mode === 'focused' ? [targets[0]] : [],
    partial_competencies: mode === 'focused' ? [targets[1]] : [],
    topics: targets,
  }
}
function audioPath(exerciseId: string) {
  let hash = 2166136261
  for (const char of exerciseId)
    hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
  return `/task-audio/demo_${(hash >>> 0).toString(16)}/file`
}

export async function mockTrainingResponse(
  userId: string,
  path: string,
  method: string,
  body: Record<string, unknown>,
  options: RequestInit,
  search: string,
  diagnostic: (id: string) => DiagnosticProgress | undefined,
): Promise<Response | undefined> {
  const previewMatch = path.match(
    /^\/api\/v1\/diagnostic-sessions\/([^/]+)\/training\/preview$/,
  )
  const diagnosticTrainingMatch = path.match(
    /^\/api\/v1\/diagnostic-sessions\/([^/]+)\/training$/,
  )
  const sessionMatch = path.match(
    /^\/api\/v1\/training-sessions\/([^/]+)(?:\/(answers|history|current\/audio))?$/,
  )
  if (previewMatch) {
    const id = decodeURIComponent(previewMatch[1])
    const progress = diagnostic(id)
    if (!progress || progress.status !== 'completed')
      return failed('DIAGNOSTIC_NOT_COMPLETED', 404)
    return json(preview(id, 'focused'))
  }
  if (diagnosticTrainingMatch) {
    const diagnosticId = decodeURIComponent(diagnosticTrainingMatch[1])
    const progress = diagnostic(diagnosticId)
    if (!progress || progress.status !== 'completed')
      return failed('DIAGNOSTIC_NOT_COMPLETED', 404)
    if (method === 'GET') {
      const existing = sessionByDiagnostic.get(`${userId}:${diagnosticId}`)
      const session = existing && sessions.get(existing)
      return session
        ? json(current(session))
        : failed('TRAINING_NOT_FOUND', 404)
    }
    if (method === 'POST') {
      const key = new Headers(options.headers).get('Idempotency-Key')
      if (!key) return failed('IDEMPOTENCY_KEY_REQUIRED', 422)
      const mode =
        typeof body.plan_revision === 'number' ? 'free_practice' : 'focused'
      if (mode === 'free_practice' && body.plan_revision !== 1)
        return failed('PLAN_CHANGED', 409)
      const identity = `${userId}:${diagnosticId}:${key}`
      const startedId = starts.get(identity)
      if (startedId) return json(current(sessions.get(startedId)!))
      const oldId = sessionByDiagnostic.get(`${userId}:${diagnosticId}`)
      if (oldId) return json(current(sessions.get(oldId)!))
      const session: TrainingSession = {
        userId,
        diagnosticId,
        id: crypto.randomUUID(),
        mode,
        targets:
          mode === 'focused'
            ? targets.map((target) => ({ ...target }))
            : targets.map((target) => ({ ...target })),
        answers: [],
        exercise: exerciseFor(1, targets[0]),
        accepted: new Map(),
      }
      sessions.set(session.id, session)
      sessionByDiagnostic.set(`${userId}:${diagnosticId}`, session.id)
      starts.set(identity, session.id)
      return json(current(session), 201)
    }
  }
  if (!sessionMatch) return undefined
  const sessionId = decodeURIComponent(sessionMatch[1])
  const session = sessions.get(sessionId)
  if (!session || session.userId !== userId) return failed('NOT_FOUND', 404)
  const operation = sessionMatch[2]
  if (!operation && method === 'GET') return json(current(session))
  if (operation === 'answers' && method === 'POST') {
    const form = options.body instanceof FormData ? options.body : undefined
    const exerciseId = form?.get('exercise_id')
    const audio = form?.get('audio')
    const key = new Headers(options.headers).get('Idempotency-Key')
    if (
      typeof exerciseId !== 'string' ||
      !key ||
      !(audio instanceof Blob) ||
      !audio.size
    )
      return failed('INVALID_ANSWER', 422)
    const audioBytes = new Uint8Array(await audio.arrayBuffer())
    const replay = session.accepted.get(key)
    if (replay)
      return replay.exerciseId === exerciseId &&
        replay.audioType === audio.type &&
        replay.audioName === (audio instanceof File ? audio.name : '') &&
        replay.audioBytes.length === audioBytes.length &&
        replay.audioBytes.every((byte, index) => byte === audioBytes[index])
        ? json(replay.result)
        : failed('IDEMPOTENCY_KEY_REUSED', 409)
    if (exerciseId !== session.exercise.exercise_id)
      return failed('EXERCISE_CHANGED', 409)
    const index = session.answers.length % session.targets.length
    const round = session.answers.length + 1
    const now = Math.floor(Date.now() / 1000)
    const attempt: TrainingAttempt = {
      sequence: session.answers.length + 1,
      exercise_id: exerciseId,
      round,
      target_index: index,
      transcription_id: crypto.randomUUID(),
      text: 'Это классификация, потому что результат относится к одному из двух классов.',
      score: 2,
      max_score: 2,
      verdict: 'correct',
      criterion_results: [
        {
          key: 'choice',
          satisfied: true,
          explanation: 'Тип задачи назван верно.',
        },
      ],
      feedback: [
        'Ответ верный.',
        'Вы правильно определили тип задачи.',
        'Продолжайте применять этот критерий.',
      ],
      created_at: now,
    }
    session.answers.push(attempt)
    session.exercise = exerciseFor(
      session.answers.length + 1,
      session.targets[session.answers.length % session.targets.length],
    )
    const result = current(session)
    session.accepted.set(key, {
      result,
      exerciseId,
      audioBytes,
      audioType: audio.type,
      audioName: audio instanceof File ? audio.name : '',
    })
    return json(result)
  }
  if (operation === 'history' && method === 'GET') {
    const query = new URLSearchParams(search)
    const limit = Math.max(1, Math.min(100, Number(query.get('limit')) || 20))
    const offset = Math.max(0, Number(query.get('cursor')) || 0)
    const items = session.answers.slice(offset, offset + limit)
    const next = offset + items.length
    return json({
      items,
      targets: session.targets,
      ...(next < session.answers.length ? { next_cursor: String(next) } : {}),
    })
  }
  if (operation === 'current/audio' && method === 'GET') {
    const query = new URLSearchParams(search)
    const exerciseId = query.get('exercise_id')
    if (!exerciseId) return failed('INVALID_EXERCISE', 422)
    return json({
      exercise_id: exerciseId,
      status: 'ready',
      audio_url: audioPath(exerciseId),
    })
  }
  return undefined
}

export function mockTrainingAudio() {
  return new Response(createDemoAudio(), {
    headers: { 'Content-Type': 'audio/wav' },
  })
}
