import { i18n } from '@/shared/i18n'
import { createDemoAudio } from '@/shared/lib'
import { AssessmentApiError } from '@/entities/assessment'
import type {
  Assessment,
  DemoAssessmentTask,
  OverallFeedbackItemInput,
} from '@/entities/assessment'
import { isMockApi } from '@/shared/api'
import {
  mockDiagnosticResponse,
  mockDiagnosticForSubject,
  readMockDiagnostic,
  resetMockDiagnostic,
} from './mock-diagnostic-api'
import {
  mockTrainingAudio,
  mockTrainingResponse,
  mockTrainingSessionForDiagnostic,
  resetMockTraining,
} from './mock-training-api'
import { mockDiagnosticStorage } from './mock-diagnostic-storage'

type DemoUser = {
  id: string
  email: string
  display_name: string
  role: 'student'
  created_at: number
}
const demoAccount: DemoUser = {
  id: 'demo-student',
  email: 'student@example.com',
  display_name: i18n.t('mockApi.userName'),
  role: 'student',
  created_at: 1791158400,
}
const demoPassword = 'demo-student-2026'
let user: DemoUser | null = null
const transcriptions = new Map<string, string>()

function pause(ms: number, signal?: AbortSignal | null) {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason)
      return
    }
    const abort = () => {
      clearTimeout(timer)
      reject(signal?.reason)
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', abort)
      resolve()
    }, ms)
    signal?.addEventListener('abort', abort, { once: true })
  })
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
function error(status: number, code: string, key: string) {
  return json({ code, message: i18n.t(`mockApi.${key}`), details: [] }, status)
}

export async function mockApiFetch(
  url: string,
  options: RequestInit = {},
): Promise<Response> {
  const requestUrl = new URL(url, 'http://mock.local')
  const path = requestUrl.pathname
  const method = options.method ?? 'GET'
  await pause(
    path.endsWith('/voice/transcriptions') ? 600 : 100,
    options.signal,
  )
  let body: Record<string, unknown> = {}
  if (typeof options.body === 'string') {
    try {
      body = JSON.parse(options.body)
    } catch {
      return error(422, 'VALIDATION_ERROR', 'invalid')
    }
    if (!body || typeof body !== 'object')
      return error(422, 'VALIDATION_ERROR', 'invalid')
  }
  if (method === 'GET' && path.endsWith('/auth/me'))
    return user ? json(user) : error(401, 'UNAUTHORIZED', 'unauthorized')
  if (method === 'POST' && /\/auth\/(login|register)$/.test(path)) {
    const emptyDemoLogin =
      path.endsWith('/login') && body.email === '' && body.password === ''
    if (
      !emptyDemoLogin &&
      (typeof body.email !== 'string' ||
        !body.email.trim() ||
        typeof body.password !== 'string' ||
        !body.password)
    )
      return error(422, 'VALIDATION_ERROR', 'invalid')
    if (path.endsWith('/register'))
      return error(422, 'VALIDATION_ERROR', 'registrationUnavailable')
    if (
      !emptyDemoLogin &&
      (typeof body.email !== 'string' ||
        body.email.trim().toLowerCase() !== demoAccount.email ||
        body.password !== demoPassword)
    )
      return error(401, 'UNAUTHORIZED', 'invalidCredentials')
    user = { ...demoAccount }
    transcriptions.clear()
    resetMockDiagnostic()
    resetMockTraining()
    mockDiagnosticStorage.clear()
    const now = Math.floor(Date.now() / 1000)
    return json(
      {
        user,
        session: {
          access_expires_at: now + 900,
          refresh_expires_at: now + 2592000,
        },
      },
      path.endsWith('/register') ? 201 : 200,
    )
  }
  if (method === 'POST' && path.endsWith('/auth/logout')) {
    user = null
    transcriptions.clear()
    resetMockDiagnostic()
    resetMockTraining()
    mockDiagnosticStorage.clear()
    return new Response(null, { status: 204 })
  }
  if (!user) return error(401, 'UNAUTHORIZED', 'unauthorized')
  const mockUser = user
  if (method === 'POST' && path.endsWith('/auth/refresh')) {
    const now = Math.floor(Date.now() / 1000)
    return json({
      access_expires_at: now + 900,
      refresh_expires_at: now + 2592000,
    })
  }
  if (method === 'POST' && path.endsWith('/voice/transcriptions')) {
    const audio =
      options.body instanceof FormData ? options.body.get('audio') : null
    if (!(audio instanceof Blob) || !audio.size)
      return error(422, 'INVALID_AUDIO', 'invalid')
    const id = crypto.randomUUID()
    const text = i18n.t('mockApi.transcript')
    transcriptions.set(id, text)
    return json({ id, text, created_at: Math.floor(Date.now() / 1000) })
  }
  if (method === 'GET' && path.endsWith('/subjects'))
    return json([
      { id: 'subject:intro-to-ml', name: 'Введение в ML', ready: true },
    ])
  const learningStateMatch = path.match(/\/subjects\/([^/]+)\/learning-state$/)
  if (learningStateMatch && method === 'GET') {
    const subjectId = decodeURIComponent(learningStateMatch[1])
    if (subjectId !== 'subject:intro-to-ml')
      return error(404, 'SUBJECT_NOT_FOUND', 'notFound')
    const known = mockDiagnosticForSubject(mockUser.id, subjectId)
    const completedDiagnostic =
      known?.status === 'completed' ? known.session_id : undefined
    return json({
      subject_id: subjectId,
      subject_name: 'Введение в ML',
      diagnostic_status: known?.status ?? 'not_started',
      diagnostic_completed: !!completedDiagnostic,
      training_available: !!completedDiagnostic,
      ...(completedDiagnostic
        ? { diagnostic_session_id: completedDiagnostic }
        : {}),
      ...(completedDiagnostic
        ? {
            active_session_id: mockTrainingSessionForDiagnostic(
              mockUser.id,
              completedDiagnostic,
            ),
          }
        : {}),
    })
  }
  const diagnosticResponse = mockDiagnosticResponse(
    mockUser.id,
    path,
    method,
    body,
    options,
  )
  if (diagnosticResponse) return diagnosticResponse
  const trainingResponse = await mockTrainingResponse(
    mockUser.id,
    path,
    method,
    body,
    options,
    requestUrl.search.slice(1),
    (id) => readMockDiagnostic(mockUser.id, id),
  )
  if (trainingResponse) return trainingResponse
  if (
    method === 'GET' &&
    /\/diagnostic-sessions\/[^/]+\/current\/audio$/.test(path)
  ) {
    const sessionId = path.match(
      /\/diagnostic-sessions\/([^/]+)\/current\/audio$/,
    )?.[1]
    if (
      !sessionId ||
      !readMockDiagnostic(mockUser.id, decodeURIComponent(sessionId))
    )
      return error(404, 'NOT_FOUND', 'notFound')
    const taskId = requestUrl.searchParams.get('variant_task_id')
    if (!taskId) return error(422, 'VALIDATION_ERROR', 'invalid')
    let hash = 2166136261
    for (const char of taskId)
      hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
    const audioId = `demo_${(hash >>> 0).toString(16)}`
    return json({
      variant_task_id: taskId,
      status: 'ready',
      audio_url: `/task-audio/${audioId}/file`,
    })
  }
  if (
    method === 'POST' &&
    /\/diagnostic-sessions\/[^/]+\/current\/audio\/regenerate$/.test(path)
  ) {
    const sessionId = path.match(
      /\/diagnostic-sessions\/([^/]+)\/current\/audio\/regenerate$/,
    )?.[1]
    if (
      !sessionId ||
      !readMockDiagnostic(mockUser.id, decodeURIComponent(sessionId))
    )
      return error(404, 'NOT_FOUND', 'notFound')
    const taskId = body.variant_task_id
    if (typeof taskId !== 'string' || !taskId)
      return error(422, 'VALIDATION_ERROR', 'invalid')
    let hash = 2166136261
    for (const char of taskId)
      hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
    const audioId = `demo_${(hash >>> 0).toString(16)}`
    return json({
      variant_task_id: taskId,
      status: 'ready',
      audio_url: `/task-audio/${audioId}/file`,
    })
  }
  if (method === 'GET' && /\/task-audio\/[A-Za-z0-9_-]+\/file$/.test(path)) {
    if (path.includes('/demo_')) return mockTrainingAudio()
    return new Response(createDemoAudio(), {
      headers: { 'Content-Type': 'audio/wav' },
    })
  }
  if (method === 'POST' && path.endsWith('/assessments/overall-feedback')) {
    const rawAnswers = (
      Array.isArray(body.answers) ? body.answers : []
    ) as OverallFeedbackItemInput[]
    const totalScore = rawAnswers.reduce(
      (sum: number, a) => sum + (typeof a?.score === 'number' ? a.score : 0),
      0,
    )
    const totalMax =
      rawAnswers.reduce(
        (sum: number, a) =>
          sum + (typeof a?.max_score === 'number' ? a.max_score : 2),
        0,
      ) || 14
    const pct = Math.round((totalScore * 100) / totalMax)
    const topicOf = (a: OverallFeedbackItemInput) =>
      a.topic || a.question || 'Тема'
    return json({
      score: totalScore,
      max_score: totalMax,
      score_percentage: pct,
      summary:
        pct >= 80
          ? `Отличный результат! Вы набрали ${totalScore} из ${totalMax} баллов (${pct}%). Продемонстрировано уверенное понимание всех тем среза.`
          : `Хороший результат: ${totalScore} из ${totalMax} баллов (${pct}%). Рекомендуется закрепить темы с неполным баллом.`,
      strengths: rawAnswers
        .filter((a) => a.score === a.max_score)
        .map(
          (a) =>
            `Тема «${topicOf(a)}»: уверенный ответ (${a.score}/${a.max_score})`,
        ),
      gaps: rawAnswers
        .filter((a) => a.score === 0)
        .map(
          (a) =>
            `Тема «${topicOf(a)}»: выявлен пробел (0/${a.max_score || 2}). Пояснение: ${Array.isArray(a.feedback) ? a.feedback.join('; ') : 'неверный выбор'}`,
        ),
      partials: rawAnswers
        .filter((a) => a.score > 0 && a.score < a.max_score)
        .map(
          (a) =>
            `Тема «${topicOf(a)}»: частичное понимание (${a.score}/${a.max_score})`,
        ),
      recommendations: [
        ...rawAnswers
          .filter((a) => a.score === 0)
          .map(
            (a) =>
              `Повторить тему «${topicOf(a)}» и разобрать критерии решения.`,
          ),
        ...rawAnswers
          .filter((a) => a.score > 0 && a.score < a.max_score)
          .map(
            (a) => `Закрепить практическое применение в теме «${topicOf(a)}».`,
          ),
      ],
      generated_at: Math.floor(Date.now() / 1000),
    })
  }
  return error(404, 'NOT_FOUND', 'notFound')
}

// Local demo grading never calls the saved-variant assessment endpoint.
export async function demoAssessment(
  transcriptionId: string,
  signal: AbortSignal,
): Promise<Assessment> {
  await pause(700, signal)
  if (!user) throw new AssessmentApiError(401)
  if (!transcriptions.has(transcriptionId)) throw new AssessmentApiError(404)
  return {
    score: 2,
    maxScore: 2,
    verdict: 'correct',
    criterionResults: [
      {
        key: 'demo',
        satisfied: true,
        explanation: i18n.t('mockApi.feedback2'),
      },
    ],
    feedback: ['feedback1', 'feedback2', 'feedback3'].map((key) =>
      i18n.t(`mockApi.${key}`),
    ) as [string, string, string],
  }
}

export async function evaluateDemoAnswer(
  transcriptionId: string,
  _task: DemoAssessmentTask,
  signal: AbortSignal,
): Promise<Assessment> {
  if (!isMockApi()) throw new Error('Demo assessment requires mock mode')
  return demoAssessment(transcriptionId, signal)
}
