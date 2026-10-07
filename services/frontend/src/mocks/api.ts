import { i18n } from '../i18n/i18n'
import { createDemoAudio } from '../platform/mock-audio'

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
  const path = new URL(url, 'http://mock.local').pathname
  const method = options.method ?? 'GET'
  await pause(
    path.endsWith('/voice/transcriptions')
      ? 600
      : path.endsWith('/assessments/evaluate')
        ? 700
        : 100,
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
    return new Response(null, { status: 204 })
  }
  if (!user) return error(401, 'UNAUTHORIZED', 'unauthorized')
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
  if (method === 'POST' && path.endsWith('/voice/syntheses')) {
    if (typeof body.text !== 'string' || !body.text.trim())
      return error(422, 'VALIDATION_ERROR', 'invalid')
    return new Response(createDemoAudio(), {
      headers: { 'Content-Type': 'audio/wav' },
    })
  }
  if (method === 'POST' && path.endsWith('/assessments/evaluate')) {
    if (
      typeof body.transcription_id !== 'string' ||
      !transcriptions.has(body.transcription_id)
    )
      return error(404, 'NOT_FOUND', 'notFound')
    if (typeof body.task_id !== 'string' || !body.task_id.trim())
      return error(422, 'VALIDATION_ERROR', 'invalid')
    return json({
      score: 2,
      verdict: 'correct',
      criterion_results: [
        {
          key: 'demo',
          satisfied: true,
          explanation: 'Демонстрационный критерий выполнен.',
        },
      ],
      feedback: ['feedback1', 'feedback2', 'feedback3'].map((key) =>
        i18n.t(`mockApi.${key}`),
      ),
    })
  }
  if (method === 'POST' && path.endsWith('/assessments/overall-feedback')) {
    const rawAnswers = Array.isArray(body.answers) ? body.answers : []
    const totalScore = rawAnswers.reduce((sum: number, a: any) => sum + (typeof a?.score === 'number' ? a.score : 0), 0)
    const totalMax = rawAnswers.reduce((sum: number, a: any) => sum + (typeof a?.max_score === 'number' ? a.max_score : 2), 0) || 14
    const pct = Math.round((totalScore * 100) / totalMax)
    const topicOf = (a: any) => a.topic || a.question || 'Тема'
    return json({
      score: totalScore,
      max_score: totalMax,
      score_percentage: pct,
      summary: pct >= 80
        ? `Отличный результат! Вы набрали ${totalScore} из ${totalMax} баллов (${pct}%). Продемонстрировано уверенное понимание всех тем среза.`
        : `Хороший результат: ${totalScore} из ${totalMax} баллов (${pct}%). Рекомендуется закрепить темы с неполным баллом.`,
      strengths: rawAnswers.filter((a: any) => a.score === a.max_score).map((a: any) => `Тема «${topicOf(a)}»: уверенный ответ (${a.score}/${a.max_score})`),
      gaps: rawAnswers.filter((a: any) => a.score === 0).map((a: any) => `Тема «${topicOf(a)}»: выявлен пробел (0/${a.max_score || 2}). Пояснение: ${Array.isArray(a.feedback) ? a.feedback.join('; ') : 'неверный выбор'}`),
      partials: rawAnswers.filter((a: any) => a.score > 0 && a.score < a.max_score).map((a: any) => `Тема «${topicOf(a)}»: частичное понимание (${a.score}/${a.max_score})`),
      recommendations: [
        ...rawAnswers.filter((a: any) => a.score === 0).map((a: any) => `Повторить тему «${topicOf(a)}» и разобрать критерии решения.`),
        ...rawAnswers.filter((a: any) => a.score > 0 && a.score < a.max_score).map((a: any) => `Закрепить практическое применение в теме «${topicOf(a)}».`),
      ],
      generated_at: Math.floor(Date.now() / 1000),
    })
  }
  return error(404, 'NOT_FOUND', 'notFound')
}
