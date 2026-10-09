import type {
  DiagnosticAnswer,
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'

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
  answer?: DiagnosticAnswer
  accepted?: DiagnosticProgress
  answerKey?: string
}

const variants = new Map<string, string>()
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
  path: string,
  method: string,
  body: Record<string, unknown>,
  options: RequestInit,
): Response | undefined {
  if (path.endsWith('/variants') && method === 'POST') {
    const key =
      new Headers(options.headers).get('Idempotency-Key') ?? crypto.randomUUID()
    let id = variants.get(key)
    if (!id) {
      id = crypto.randomUUID()
      variants.set(key, id)
    }
    return json({ id }, 201)
  }
  if (path.endsWith('/diagnostic-sessions') && method === 'POST') {
    const variantId = body.variant_id
    if (
      typeof variantId !== 'string' ||
      ![...variants.values()].includes(variantId)
    )
      return json({ code: 'VARIANT_NOT_FOUND' }, 404)
    const key =
      new Headers(options.headers).get('Idempotency-Key') ?? crypto.randomUUID()
    const existing = starts.get(key)
    if (existing) return json(progress(sessions.get(existing)!))
    const session: Session = { id: crypto.randomUUID(), variantId }
    sessions.set(session.id, session)
    starts.set(key, session.id)
    return json(progress(session), 201)
  }
  const match = path.match(
    /\/diagnostic-sessions\/([^/]+)(?:\/(answers|result|feedback))?$/,
  )
  if (!match) return undefined
  const session = sessions.get(match[1])
  if (!session) return json({ code: 'NOT_FOUND' }, 404)
  const operation = match[2]
  if (!operation && method === 'GET') return json(progress(session))
  if (operation === 'answers' && method === 'POST') {
    const key = new Headers(options.headers).get('Idempotency-Key')
    if (session.accepted)
      return key === session.answerKey
        ? json(session.accepted)
        : json({ code: 'ANSWER_ALREADY_ACCEPTED' }, 409)
    const form = options.body
    const audio = form instanceof FormData ? form.get('audio') : null
    if (
      !(form instanceof FormData) ||
      form.get('variant_task_id') !== task.variant_task_id ||
      !(audio instanceof Blob) ||
      !audio.size
    )
      return json({ code: 'INVALID_AUDIO' }, 422)
    const now = Math.floor(Date.now() / 1000)
    session.answer = {
      variant_task_id: task.variant_task_id,
      source_task_id: task.source_task_id,
      competency_id: task.competency_id,
      outcome_id: task.outcome_id,
      role: task.role,
      task,
      transcription_id: crypto.randomUUID(),
      text: 'Это классификация, потому что результат относится к одному из двух классов.',
      grader_score: 2,
      grader_max_score: 2,
      score: 2,
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
        'Вы правильно определили классификацию.',
        'Закрепите отличие от регрессии.',
      ],
      created_at: now,
    }
    session.answerKey = key ?? undefined
    session.accepted = {
      ...progress(session),
      text: session.answer.text,
      score: 2,
      grader_score: 2,
      grader_max_score: 2,
      verdict: 'correct',
      criterion_results: session.answer.criterion_results,
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
      diagnostic_score: 2,
      maximum_score: 2,
      answers: [session.answer],
      untested_basics: [],
    })
  }
  if (operation === 'feedback' && method === 'GET') {
    if (!session.answer) return json({ code: 'SESSION_NOT_COMPLETED' }, 409)
    return json({
      session_id: session.id,
      diagnostic_score: 2,
      maximum_score: 2,
      score_percentage: 100,
      summary: 'Диагностика показала уверенное понимание классификации.',
      strengths: ['Вы уверенно различаете задачи классификации.'],
      confirmed_gaps: [],
      partial_competencies: [],
      unverified_competencies: [],
      training_recommendations: [],
      generated_at: Math.floor(Date.now() / 1000),
    })
  }
  return undefined
}
