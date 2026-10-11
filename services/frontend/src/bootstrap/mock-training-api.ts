import { createDemoAudio } from '@/shared/lib'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'
import type {
  TrainingAttempt,
  TrainingExercise,
  TrainingPreview,
  TrainingProgress,
  TrainingTarget,
} from '@/entities/training'
import type {
  TrainingChatCheck,
  TrainingChatMessage,
  TrainingChatPage,
} from '@/entities/training'

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
  subjectId: string
  subjectName: string
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
type MockChat = Omit<TrainingChatPage, 'next_cursor'>
const chats = new Map<string, MockChat>()
const nextResults = new Map<
  string,
  { fingerprint: string; result: TrainingProgress }
>()

export function resetMockTraining() {
  sessions.clear()
  sessionByDiagnostic.clear()
  starts.clear()
  chats.clear()
  nextResults.clear()
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
    subject_id: session.subjectId,
    subject_name: session.subjectName,
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
  subject: { id: string; name: string },
  mode: 'focused' | 'free_practice',
): TrainingPreview {
  return {
    diagnostic_session_id: diagnosticId,
    subject_id: subject.id,
    subject_name: subject.name,
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

function chatFor(session: TrainingSession, exerciseId: string): MockChat {
  let chat = chats.get(exerciseId)
  if (!chat) {
    const welcome: TrainingChatMessage = {
      id: crypto.randomUUID(),
      sequence: 1,
      role: 'assistant',
      kind: 'welcome',
      text: 'Давайте разберём условие вместе. Что уже удалось понять?',
      created_at: Math.floor(Date.now() / 1000),
    }
    chat = {
      session_id: session.id,
      exercise_id: exerciseId,
      revision: 0,
      status: 'active',
      items: [welcome],
      latest_check: null,
      pending_turn: null,
    }
    chats.set(exerciseId, chat)
  }
  return chat
}

export async function mockTrainingResponse(
  userId: string,
  path: string,
  method: string,
  body: Record<string, unknown>,
  options: RequestInit,
  search: string,
  diagnostic: (id: string) => DiagnosticProgress | undefined,
  diagnosticSubject: (id: string) => { id: string; name: string } | undefined,
): Promise<Response | undefined> {
  const previewMatch = path.match(
    /^\/api\/v1\/diagnostic-sessions\/([^/]+)\/training\/preview$/,
  )
  const diagnosticTrainingMatch = path.match(
    /^\/api\/v1\/diagnostic-sessions\/([^/]+)\/training$/,
  )
  const sessionMatch = path.match(
    /^\/api\/v1\/training-sessions\/([^/]+)(?:\/(answers(?:\/reset)?|skip|history|current\/audio))?$/,
  )
  const chatMatch = path.match(
    /^\/api\/v1\/training-sessions\/([^/]+)\/exercises\/([^/]+)\/chat(?:\/turns(?:\/([^/]+)\/(retry|cancel))?)?$/,
  )
  const nextMatch = path.match(
    /^\/api\/v1\/training-sessions\/([^/]+)\/exercises\/([^/]+)\/next$/,
  )
  if (previewMatch) {
    const id = decodeURIComponent(previewMatch[1])
    const progress = diagnostic(id)
    const subject = diagnosticSubject(id)
    if (!progress || progress.status !== 'completed' || !subject)
      return failed('DIAGNOSTIC_NOT_COMPLETED', 404)
    return json(preview(id, subject, 'focused'))
  }
  if (diagnosticTrainingMatch) {
    const diagnosticId = decodeURIComponent(diagnosticTrainingMatch[1])
    const progress = diagnostic(diagnosticId)
    const subject = diagnosticSubject(diagnosticId)
    if (!progress || progress.status !== 'completed' || !subject)
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
        subjectId: subject.id,
        subjectName: subject.name,
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
  if (!sessionMatch && !chatMatch && !nextMatch) return undefined
  const sessionId = decodeURIComponent(
    (sessionMatch ?? chatMatch ?? nextMatch)![1],
  )
  const session = sessions.get(sessionId)
  if (!session || session.userId !== userId) return failed('NOT_FOUND', 404)
  if (chatMatch) {
    const exerciseId = decodeURIComponent(chatMatch[2])
    const chat = chatFor(session, exerciseId)
    if (method === 'GET' && !chatMatch[3]) return json(chat)
    if (method === 'POST' && chatMatch[4] === 'cancel')
      return new Response(null, { status: 204 })
    if (method === 'POST' && chatMatch[4] === 'retry')
      return failed('CHAT_TURN_IN_PROGRESS', 409)
    if (method !== 'POST' || chatMatch[3]) return failed('NOT_FOUND', 404)
    const key = new Headers(options.headers).get('Idempotency-Key')
    const form = options.body instanceof FormData ? options.body : undefined
    const revision = Number(form?.get('revision') ?? body.revision)
    const kind = String(form?.get('kind') ?? body.kind)
    const source = form ? 'voice' : 'text'
    const text =
      source === 'voice'
        ? revision === 0
          ? 'Демонстрационная голосовая реплика: объясните свой выбор.'
          : 'Классификация, потому что модель выбирает один из классов.'
        : String(body.text ?? '')
    if (
      !key ||
      !['message', 'check'].includes(kind) ||
      !Number.isInteger(revision) ||
      revision < 0 ||
      !text.trim()
    )
      return failed('VALIDATION_ERROR', 422)
    const chatKind = kind as 'message' | 'check'
    if (exerciseId !== session.exercise.exercise_id || chat.status !== 'active')
      return failed('TRAINING_EXERCISE_NOT_CURRENT', 409)
    if (revision !== chat.revision) return failed('CHAT_REVISION_CONFLICT', 409)
    const score: 0 | 1 | 2 = /классификац|classification/i.test(text)
      ? 2
      : /класс|class/i.test(text)
        ? 1
        : 0
    const verdict =
      score === 2 ? 'correct' : score === 1 ? 'partial' : 'incorrect'
    const answerReady =
      chatKind === 'check' ||
      /потому|так как|выбирает один из классов/i.test(text)
    const check: TrainingChatCheck | null = answerReady
      ? {
          revision: chat.revision + 1,
          final_answer: text,
          source,
          score,
          max_score: 2,
          verdict,
        }
      : null
    const now = Math.floor(Date.now() / 1000)
    const userMessage: TrainingChatMessage = {
      id: crypto.randomUUID(),
      sequence: chat.revision * 2 + 2,
      role: 'user',
      kind: chatKind,
      source,
      text,
      created_at: now,
    }
    const assistantMessage: TrainingChatMessage = {
      id: crypto.randomUUID(),
      sequence: chat.revision * 2 + 3,
      role: 'assistant',
      kind: chatKind,
      text: answerReady
        ? score === 2
          ? 'Вы связали ответ с условием. Хотите уточнить ещё один признак?'
          : 'Пока не все требования выполнены. Какой признак поможет уточнить ответ?'
        : 'Вы назвали важную часть рассуждения. Какое свойство результата подтверждает ваш выбор?',
      created_at: now,
    }
    chat.items.push(userMessage, assistantMessage)
    chat.revision++
    chat.latest_check = check
    chat.pending_turn = null
    const turnId = crypto.randomUUID()
    return json({
      session_id: session.id,
      exercise_id: exerciseId,
      revision: chat.revision,
      turn_id: turnId,
      messages: [userMessage, assistantMessage],
      latest_check: check,
    })
  }
  if (nextMatch && method === 'POST') {
    const exerciseId = decodeURIComponent(nextMatch[2])
    const key = new Headers(options.headers).get('Idempotency-Key')
    const revision = body.revision
    if (!key || !Number.isInteger(revision) || Number(revision) < 0)
      return failed('VALIDATION_ERROR', 422)
    const identity = `${session.id}:${key}`
    const fingerprint = `${exerciseId}:${revision}`
    const replay = nextResults.get(identity)
    if (replay)
      return replay.fingerprint === fingerprint
        ? json(replay.result)
        : failed('IDEMPOTENCY_KEY_REUSED', 409)
    const chat = chatFor(session, exerciseId)
    if (exerciseId !== session.exercise.exercise_id)
      return failed('TRAINING_EXERCISE_NOT_CURRENT', 409)
    if (
      chat.status !== 'active' ||
      chat.revision !== revision ||
      chat.latest_check?.revision !== revision
    )
      return failed('CHAT_CHECK_REQUIRED', 409)
    const check = chat.latest_check
    const index = session.answers.length % session.targets.length
    const attempt: TrainingAttempt = {
      sequence: session.answers.length + 1,
      exercise_id: exerciseId,
      round: session.answers.length + 1,
      target_index: index,
      text: check.final_answer,
      answer_source: check.source,
      ...(check.transcription_id
        ? { transcription_id: check.transcription_id }
        : {}),
      score: check.score,
      max_score: 2,
      verdict: check.verdict,
      criterion_results: [
        {
          key: 'understanding',
          satisfied: check.score === 2,
          explanation:
            check.score === 2
              ? 'Связал ответ с условием.'
              : 'Ответ можно уточнить.',
        },
      ],
      feedback:
        check.score === 2
          ? [
              'Ответ верный.',
              'Вы правильно применили понятие.',
              'Продолжайте так же.',
            ]
          : [
              'Ответ частично верный.',
              'Уточните связь с условием.',
              'Попробуйте ещё раз.',
            ],
      created_at: Math.floor(Date.now() / 1000),
    }
    session.answers.push(attempt)
    session.targets[index] = trainingTarget(
      session.targets[index]!,
      check.score,
    )
    session.exercise = exerciseFor(
      session.answers.length + 1,
      session.targets[session.answers.length % session.targets.length]!,
    )
    chat.status = 'closed'
    const result = { ...current(session), answer: attempt }
    nextResults.set(identity, { fingerprint, result })
    return json(result)
  }
  if (!sessionMatch) return undefined
  const operation = sessionMatch[2]
  if (!operation && method === 'GET') return json(current(session))
  if ((operation === 'answers' || operation === 'skip') && method === 'POST') {
    const form = options.body instanceof FormData ? options.body : undefined
    const exerciseId =
      operation === 'skip' ? body.exercise_id : form?.get('exercise_id')
    const audio = form?.get('audio')
    const key = new Headers(options.headers).get('Idempotency-Key')
    if (
      typeof exerciseId !== 'string' ||
      !key ||
      (operation !== 'skip' && (!(audio instanceof Blob) || !audio.size))
    )
      return failed('INVALID_ANSWER', 422)
    const skipped = operation === 'skip'
    const audioBytes = skipped
      ? new Uint8Array()
      : new Uint8Array(await (audio as Blob).arrayBuffer())
    const replay = session.accepted.get(key)
    if (replay)
      return replay.exerciseId === exerciseId &&
        replay.audioType === (skipped ? 'skip' : (audio as Blob).type) &&
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
      skipped,
      sequence: session.answers.length + 1,
      exercise_id: exerciseId,
      round,
      target_index: index,
      transcription_id: crypto.randomUUID(),
      text: skipped
        ? 'Я не знаю. Пропустить'
        : 'Это классификация, потому что результат относится к одному из двух классов.',
      score: skipped ? 0 : 2,
      max_score: 2,
      verdict: skipped ? 'incorrect' : 'correct',
      criterion_results: skipped
        ? []
        : [
            {
              key: 'choice',
              satisfied: true,
              explanation: 'Тип задачи назван верно.',
            },
          ],
      feedback: skipped
        ? [
            'Вопрос пропущен.',
            'Ответ оценён в 0 баллов.',
            'Продолжите со следующим вопросом.',
          ]
        : [
            'Ответ верный.',
            'Вы правильно определили тип задачи.',
            'Продолжайте применять этот критерий.',
          ],
      created_at: now,
    }
    session.answers.push(attempt)
    const completedChat = chats.get(exerciseId)
    if (completedChat) completedChat.status = 'closed'
    session.exercise = exerciseFor(
      session.answers.length + 1,
      session.targets[session.answers.length % session.targets.length],
    )
    const result = current(session)
    session.accepted.set(key, {
      result,
      exerciseId,
      audioBytes,
      audioType: skipped ? 'skip' : (audio as Blob).type,
      audioName: audio instanceof File ? audio.name : '',
    })
    return json(result)
  }
  if (operation === 'answers/reset' && method === 'POST')
    return new Response(null, { status: 204 })
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
