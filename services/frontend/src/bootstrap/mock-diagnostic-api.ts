import type {
  DiagnosticAnswer,
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'
import { findMockSubject } from './mock-subject-catalog'

const task: DiagnosticTask = {
  variant_task_id: 'demo-main',
  source_task_id: 'demo-task',
  role: 'main',
  competency_id: 'demo-competency',
  competency_name: 'Виды задач машинного обучения',
  constituent_id: 'demo-constituent',
  constituent_name: 'Определение типа задачи',
  outcome_id: 'demo-outcome',
  outcome_name: 'Различать классификацию и регрессию',
  question: 'Демонстрационный вопрос о классификации',
  options: ['Классификация', 'Регрессия'],
  voice_instruction: 'Назовите тип задачи и объясните свой выбор.',
}

type Session = {
  id: string
  variantId: string
  subjectId: string
  userId: string
  answer?: DiagnosticAnswer
  accepted?: DiagnosticProgress
  answerKey?: string
}

const variants = new Map<
  string,
  { id: string; userId: string; subjectId: string }
>()
const sessions = new Map<string, Session>()
const starts = new Map<string, string>()

export function resetMockDiagnostic() {
  variants.clear()
  sessions.clear()
  starts.clear()
}

function json(body: unknown, status = 200) {
  return Response.json(body, { status })
}

function progress(session: Session): DiagnosticProgress {
  return session.answer
    ? {
        session_id: session.id,
        status: 'completed',
        completed_tasks: 1,
        skipped_tasks: 0,
        total_tasks: 1,
      }
    : {
        session_id: session.id,
        status: 'active',
        completed_tasks: 0,
        skipped_tasks: 0,
        total_tasks: 1,
        current: task,
      }
}

export function mockDiagnosticResponse(
  userId: string,
  path: string,
  method: string,
  body: Record<string, unknown>,
  options: RequestInit,
): Response | undefined {
  if (path.endsWith('/variants') && method === 'POST') {
    const key =
      new Headers(options.headers).get('Idempotency-Key') ?? crypto.randomUUID()
    const subjectId = body.subject_id
    if (typeof subjectId !== 'string' || !findMockSubject(subjectId))
      return json({ code: 'SUBJECT_NOT_FOUND' }, 404)
    const identity = `${userId}:${key}`
    let variant = variants.get(identity)
    if (!variant) {
      variant = {
        id: crypto.randomUUID(),
        userId,
        subjectId,
      }
      variants.set(identity, variant)
    }
    return json({ id: variant.id }, 201)
  }
  if (path.endsWith('/diagnostic-sessions') && method === 'POST') {
    const variantId = body.variant_id
    if (
      typeof variantId !== 'string' ||
      ![...variants.values()].some(
        (variant) => variant.id === variantId && variant.userId === userId,
      )
    )
      return json({ code: 'VARIANT_NOT_FOUND' }, 404)
    const key =
      new Headers(options.headers).get('Idempotency-Key') ?? crypto.randomUUID()
    const existing = starts.get(`${userId}:${key}`)
    if (existing) return json(progress(sessions.get(existing)!))
    const variant = [...variants.values()].find(
      (item) => item.id === variantId,
    )!
    const session: Session = {
      id: crypto.randomUUID(),
      variantId,
      subjectId: variant.subjectId,
      userId,
    }
    sessions.set(session.id, session)
    starts.set(`${userId}:${key}`, session.id)
    return json(progress(session), 201)
  }
  const match = path.match(
    /\/diagnostic-sessions\/([^/]+)(?:\/(answers|skip|result|feedback))?$/,
  )
  if (!match) return undefined
  const session = sessions.get(match[1])
  if (!session || session.userId !== userId)
    return json({ code: 'NOT_FOUND' }, 404)
  const operation = match[2]
  if (!operation && method === 'GET') return json(progress(session))
  if ((operation === 'answers' || operation === 'skip') && method === 'POST') {
    const key = new Headers(options.headers).get('Idempotency-Key')
    if (session.accepted)
      return key === session.answerKey
        ? json(session.accepted)
        : json({ code: 'ANSWER_ALREADY_ACCEPTED' }, 409)
    const form = options.body
    const audio = form instanceof FormData ? form.get('audio') : null
    if (
      operation === 'skip'
        ? body.variant_task_id !== task.variant_task_id
        : !(form instanceof FormData) ||
          form.get('variant_task_id') !== task.variant_task_id ||
          !(audio instanceof Blob) ||
          !audio.size
    )
      return json({ code: 'INVALID_AUDIO' }, 422)
    const skipped = operation === 'skip'
    const now = Math.floor(Date.now() / 1000)
    session.answer = {
      variant_task_id: task.variant_task_id,
      source_task_id: task.source_task_id,
      competency_id: task.competency_id,
      outcome_id: task.outcome_id,
      role: task.role,
      skipped,
      task,
      transcription_id: crypto.randomUUID(),
      text: skipped
        ? 'Я не знаю. Пропустить'
        : 'Это классификация, потому что результат относится к одному из двух классов.',
      grader_score: skipped ? 0 : 2,
      grader_max_score: 2,
      score: skipped ? 0 : 2,
      verdict: skipped ? 'incorrect' : 'correct',
      criterion_results: skipped
        ? [
            {
              key: 'skipped',
              satisfied: false,
              explanation: 'Вопрос пропущен.',
            },
          ]
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
            'Вы правильно определили классификацию.',
            'Закрепите отличие от регрессии.',
          ],
      created_at: now,
    }
    session.answerKey = key ?? undefined
    session.accepted = {
      ...progress(session),
      text: session.answer.text,
      score: session.answer.score,
      grader_score: session.answer.grader_score,
      grader_max_score: 2,
      verdict: session.answer.verdict,
      criterion_results: session.answer.criterion_results,
      answer_skipped: skipped,
      feedback: session.answer.feedback,
    }
    return json(session.accepted)
  }
  if (operation === 'result' && method === 'GET') {
    if (!session.answer) return json({ code: 'SESSION_NOT_COMPLETED' }, 409)
    return json({
      session_id: session.id,
      status: 'completed',
      variant_id: session.variantId,
      map_revision: 1,
      included_competency_count: 1,
      skipped_competencies: [],
      completed_tasks: 1,
      total_tasks: 1,
      diagnostic_score: session.answer.score,
      maximum_score: 2,
      answers: [session.answer],
      untested_basics: [],
    })
  }
  if (operation === 'feedback' && method === 'GET') {
    if (!session.answer) return json({ code: 'SESSION_NOT_COMPLETED' }, 409)
    return json({
      session_id: session.id,
      diagnostic_score: session.answer.score,
      maximum_score: 2,
      score_percentage: session.answer.score * 50,
      summary: session.answer.skipped
        ? 'Вопрос пропущен и оценён в 0 баллов.'
        : 'Диагностика показала уверенное понимание классификации.',
      strengths: session.answer.skipped
        ? []
        : ['Вы уверенно различаете задачи классификации.'],
      confirmed_gaps: [],
      partial_competencies: session.answer.skipped
        ? [
            {
              competency_id: task.competency_id,
              competency_name: task.competency_name,
              details: 'Ответ «Я не знаю. Пропустить» оценён в 0 баллов.',
            },
          ]
        : [],
      unverified_competencies: [],
      training_recommendations: [],
      generated_at: Math.floor(Date.now() / 1000),
    })
  }
  return undefined
}

export function readMockDiagnostic(
  userId: string,
  id: string,
): DiagnosticProgress | undefined {
  const session = sessions.get(id)
  return session?.userId === userId ? progress(session) : undefined
}

export function mockDiagnosticSubject(userId: string, id: string) {
  const session = sessions.get(id)
  if (!session || session.userId !== userId) return undefined
  return findMockSubject(session.subjectId)
}

export function mockDiagnosticForSubject(userId: string, subjectId: string) {
  const matching = [...sessions.values()].filter(
    (item) => item.userId === userId && item.subjectId === subjectId,
  )
  const completed = [...matching].reverse().find((item) => item.answer)
  const active = [...matching].reverse().find((item) => !item.answer)
  return {
    completed: completed ? progress(completed) : undefined,
    active: active ? progress(active) : undefined,
  }
}
