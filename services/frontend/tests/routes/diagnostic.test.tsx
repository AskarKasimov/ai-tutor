import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import * as audio from '@/shared/lib'
import { validWavBlob } from '../support/audio'
import { routeTree } from '@/routeTree.gen'

vi.mock('@/shared/lib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/lib')>()),
  createVoiceWaveform: () => ({ setStream: () => {}, dispose: () => {} }),
}))
const main = {
  variant_task_id: 'v-main',
  source_task_id: 'bank-main',
  role: 'main',
  competency_id: 'c1',
  competency_name: 'Модели',
  constituent_id: 's1',
  constituent_name: 'Выбор модели',
  outcome_id: 'o1',
  outcome_name: 'Различать модели',
  question: 'Настоящий вопрос из банка',
  options: ['Первый вариант', 'Второй вариант'],
  voice_instruction: 'Назовите модель и объясните решение.',
}
const basic = {
  ...main,
  variant_task_id: 'v-basic',
  source_task_id: 'bank-basic',
  role: 'basic',
  question: 'Базовый вопрос из банка',
  options: [],
}
const basic2 = {
  ...basic,
  variant_task_id: 'v-basic2',
  source_task_id: 'bank-basic2',
  question: 'Второй базовый вопрос',
}
const progress = {
  session_id: 'session-real',
  status: 'active',
  completed_tasks: 0,
  skipped_tasks: 0,
  total_tasks: 3,
  current: main,
}
const criteria = [
  { key: 'choice', satisfied: true, explanation: 'Модель названа.' },
]
const feedback = [
  'Серверная оценка.',
  'Серверное объяснение.',
  'Серверный совет.',
]
const resultBody = {
  session_id: 'session-real',
  status: 'completed',
  variant_id: 'variant-real',
  map_revision: 2,
  included_competency_count: 1,
  skipped_competencies: [],
  completed_tasks: 1,
  total_tasks: 3,
  diagnostic_score: 2,
  maximum_score: 2,
  answers: [
    {
      variant_task_id: 'v-main',
      source_task_id: 'bank-main',
      competency_id: 'c1',
      outcome_id: 'o1',
      role: 'main',
      task: main,
      transcription_id: 'tr1',
      text: 'Настоящая расшифровка',
      grader_score: 2,
      grader_max_score: 2,
      score: 2,
      verdict: 'correct',
      criterion_results: criteria,
      feedback,
      created_at: 1791200000,
    },
  ],
  untested_basics: [basic, basic2],
}
async function home() {
  const continuing = !!sessionStorage.getItem('ai-tutor:learning-home')
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const view = render(<RouterProvider router={router} />)
  if (!continuing) {
    fireEvent.click(
      await screen.findByRole(
        'button',
        { name: 'Начать диагностику' },
        { timeout: 5000 },
      ),
    )
  }
  return view
}
function server(
  answerStatus = 200,
  totalTasks = 3,
  feedbackStatus = 200,
  strengths: string[] | null = ['Вы уверенно различаете модели.'],
) {
  const sessionProgress = { ...progress, total_tasks: totalTasks }
  let answered = false
  const requests = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/subjects'))
      return Response.json([
        { id: 'subject:test', name: 'Предмет теста', ready: true },
      ])
    if (url.endsWith('/subjects/subject%3Atest/learning-state'))
      return Response.json({
        subject_id: 'subject:test',
        subject_name: 'Предмет теста',
        diagnostic_status: 'not_started',
        diagnostic_completed: false,
        training_available: false,
      })
    if (url.endsWith('/auth/me'))
      return Response.json({
        id: 'student-real',
        email: 'student@example.com',
        display_name: 'Студент',
        role: 'student',
      })
    if (url.endsWith('/variants') && init?.method === 'POST')
      return Response.json({ id: 'variant-real' }, { status: 201 })
    if (url.endsWith('/diagnostic-sessions') && init?.method === 'POST')
      return Response.json(sessionProgress, { status: 201 })
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: new URL(url, 'http://test').searchParams.get(
          'variant_task_id',
        ),
        status: 'ready',
        audio_url: '/task-audio/audio-main/file',
      })
    if (url.endsWith('/task-audio/audio-main/file'))
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    if (url.endsWith('/skip')) {
      answered = true
      return Response.json({
        ...sessionProgress,
        status: 'active',
        current: basic,
        completed_tasks: 1,
        answer_skipped: true,
        text: 'Я не знаю. Пропустить',
        score: 0,
        grader_score: 0,
        grader_max_score: 2,
        verdict: 'incorrect',
        criterion_results: [
          { key: 'skipped', satisfied: false, explanation: 'Вопрос пропущен.' },
        ],
        feedback: [
          'Вопрос пропущен.',
          'Ответ оценён в 0 баллов.',
          'Продолжите со следующим вопросом.',
        ],
      })
    }
    if (url.endsWith('/answers')) {
      if (answerStatus !== 200) {
        answerStatus = 200
        return Response.json(
          { code: 'GRADING_FAILED', message: 'Сбой модели' },
          { status: 502 },
        )
      }
      answered = true
      return Response.json({
        ...sessionProgress,
        status: 'completed',
        current: undefined,
        completed_tasks: 1,
        skipped_tasks: totalTasks - 1,
        text: 'Настоящая расшифровка',
        score: 2,
        grader_score: 2,
        grader_max_score: 2,
        verdict: 'correct',
        criterion_results: criteria,
        feedback,
      })
    }
    if (url.endsWith('/result'))
      return Response.json({
        ...resultBody,
        total_tasks: totalTasks,
        untested_basics: resultBody.untested_basics.slice(0, totalTasks - 1),
      })
    if (url.endsWith('/feedback')) {
      if (feedbackStatus !== 200) {
        feedbackStatus = 200
        return Response.json({ code: 'FEEDBACK_FAILED' }, { status: 503 })
      }
      return Response.json({
        session_id: 'session-real',
        diagnostic_score: 2,
        maximum_score: 2,
        score_percentage: 100,
        summary: 'Итоговое педагогическое резюме.',
        strengths,
        confirmed_gaps: [],
        partial_competencies: [],
        unverified_competencies: [],
        training_recommendations: [
          {
            competency_id: 'c1',
            competency_name: 'Модели',
            outcome_id: 'o1',
            outcome_name: 'Различать модели',
            priority: 1,
            rationale: 'Закрепите выбор модели на новых примерах.',
          },
        ],
        generated_at: 1791200000,
      })
    }
    if (url.endsWith('/diagnostic-sessions/session-real'))
      return Response.json(
        answered
          ? {
              ...sessionProgress,
              status: 'completed',
              current: undefined,
              completed_tasks: 1,
              skipped_tasks: totalTasks - 1,
            }
          : sessionProgress,
      )
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', requests)
  return requests
}
beforeEach(() => {
  sessionStorage.clear()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    stop: vi
      .fn()
      .mockResolvedValue(
        new Blob(['recorded-answer'], { type: 'audio/webm;codecs=opus' }),
      ),
    dispose: vi.fn(),
  })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
it('runs real diagnostic audio, obeys skipped basics and displays the server total and transcript', async () => {
  const requests = server()
  await home()
  expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
    'Настоящий вопрос из банка',
  )
  expect(screen.getByText('Первый вариант')).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Ваш ответ' })).toBeVisible()
  expect(
    screen.getByText('Здесь появится расшифровка вашего ответа.'),
  ).toBeVisible()
  expect(screen.queryByText('Голосовая инструкция')).not.toBeInTheDocument()
  expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuemax', '3')
  expect(screen.getByLabelText('Вопрос 1: текущий')).toBeVisible()
  expect(screen.getByLabelText('Вопрос 2: ещё недоступен')).toBeVisible()
  expect(screen.getByText('Вопрос 1', { selector: 'strong' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  const feedbackParagraph = await screen.findByText(feedback.join(' '))
  expect(feedbackParagraph).toBeVisible()
  expect(feedbackParagraph.tagName).toBe('P')
  expect(screen.queryByText('Модель названа.')).not.toBeInTheDocument()
  expect(screen.queryByText(/Критерий (не )?выполнен/)).not.toBeInTheDocument()
  // The main answer scored 2/2 and both basics were skipped by the server.
  expect(screen.getByLabelText('Вопрос 1: верно')).toBeVisible()
  expect(screen.getByLabelText('Вопрос 2: пропущен')).toBeVisible()
  expect(screen.getByLabelText('Вопрос 3: пропущен')).toBeVisible()
  expect(screen.getByText('Настоящая расшифровка')).toBeVisible()
  expect(
    screen.queryByText('Здесь появится расшифровка вашего ответа.'),
  ).not.toBeInTheDocument()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Сессия завершена' }),
  ).toBeVisible()
  expect(
    screen.getByText('Диагностический балл').parentElement,
  ).toHaveTextContent('2 / 2')
  expect(
    screen.getByRole('button', { name: 'Перейти к тренировке' }),
  ).toBeVisible()
  // The task review is folded until the student opens it.
  expect(screen.queryByText('Настоящая расшифровка')).not.toBeInTheDocument()
  fireEvent.click(
    await screen.findByRole('button', { name: /Разбор по заданиям/ }),
  )
  expect(screen.getByText('Настоящая расшифровка')).toBeVisible()
  expect(screen.getByText(feedback.join(' '))).toBeVisible()
  expect(screen.queryByText('Модель названа.')).not.toBeInTheDocument()
  expect(
    await screen.findByText('Итоговое педагогическое резюме.'),
  ).toBeVisible()
  expect(screen.getByText('Вы уверенно различаете модели.')).toBeVisible()
  expect(
    screen.getByText(/Закрепите выбор модели на новых примерах/),
  ).toBeVisible()
  expect(screen.getByText('Базовый вопрос из банка')).toBeVisible()
  expect(screen.getByText(/Не проверено/)).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Перейти к тренировке' }),
  ).toBeEnabled()
  expect(
    screen.queryByRole('button', { name: 'Новая диагностика' }),
  ).not.toBeInTheDocument()
  const [url, init] = requests.mock.calls.find(([url]) =>
    url.endsWith('/answers'),
  )!
  expect(url).toBe('/api/v1/diagnostic-sessions/session-real/answers')
  expect(init?.credentials).toBe('include')
  expect(init?.headers).toMatchObject({ 'Idempotency-Key': expect.any(String) })
  const body = init?.body as FormData
  expect(body.get('variant_task_id')).toBe('v-main')
  expect(await (body.get('audio') as File).text()).toBe('recorded-answer')
  expect(
    requests.mock.calls.some(([url]) =>
      /voice\/transcriptions|assessments\/evaluate/.test(url),
    ),
  ).toBe(false)
})
it('keeps the diagnostic result visible and retries failed overall feedback', async () => {
  const requests = server(200, 3, 503)
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(
    await screen.findByText('Не удалось загрузить общий фидбэк.'),
  ).toBeVisible()
  expect(
    screen.getByText('Диагностический балл').parentElement,
  ).toHaveTextContent('2 / 2')
  fireEvent.click(
    screen.getByRole('button', { name: 'Повторить загрузку фидбэка' }),
  )
  expect(
    await screen.findByText('Итоговое педагогическое резюме.'),
  ).toBeVisible()
  expect(
    requests.mock.calls.filter(([url]) => url.endsWith('/feedback')),
  ).toHaveLength(2)
})
it('shows the task review only together with the overall feedback', async () => {
  const requests = server()
  let finish!: () => void
  const ready = new Promise<void>((resolve) => {
    finish = resolve
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/feedback')) await ready
      return requests(url, init)
    }),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Готовим ваш персональный разбор',
    }),
  ).toBeVisible()
  expect(screen.queryByText('Настоящая расшифровка')).not.toBeInTheDocument()
  finish()
  expect(
    await screen.findByText('Итоговое педагогическое резюме.'),
  ).toBeVisible()
  fireEvent.click(
    await screen.findByRole('button', { name: /Разбор по заданиям/ }),
  )
  expect(screen.getByText('Настоящая расшифровка')).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Готовим ваш персональный разбор' }),
  ).not.toBeInTheDocument()
})

it('shows overall feedback when the backend sends null strengths', async () => {
  server(200, 3, 200, null)
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(
    await screen.findByText('Итоговое педагогическое резюме.'),
  ).toBeVisible()
  expect(
    screen.queryByText('Не удалось загрузить общий фидбэк.'),
  ).not.toBeInTheDocument()
})
it('keeps the audio and idempotency key on a failed submission and retries without recording again', async () => {
  const requests = server(502)
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent('ответ')
  expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
    'Настоящий вопрос из банка',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Отправить снова' }))
  expect(await screen.findByText(/Серверное объяснение\./)).toBeVisible()
  const calls = requests.mock.calls.filter(([url]) => url.endsWith('/answers'))
  expect(calls).toHaveLength(2)
  expect(calls[0][1]?.headers).toEqual(calls[1][1]?.headers)
  expect(calls[0][1]?.body).toBe(calls[1][1]?.body)
  expect(audio.startRecording).toHaveBeenCalledTimes(1)
})
it('restores the existing session on remount without creating another variant', async () => {
  const requests = server()
  const first = await home()
  await screen.findByRole('heading', { name: 'Настоящий вопрос из банка' })
  first.unmount()
  await home()
  await screen.findByRole('heading', { name: 'Настоящий вопрос из банка' })
  expect(
    requests.mock.calls.filter(
      ([url, init]) => url.endsWith('/variants') && init?.method === 'POST',
    ),
  ).toHaveLength(1)
  expect(
    requests.mock.calls.some(([url]) =>
      url.endsWith('/diagnostic-sessions/session-real'),
    ),
  ).toBe(true)
})
it('plays the stored server instruction automatically when a task opens', async () => {
  const requests = server()
  vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  await home()
  await waitFor(() => expect(audio.playQuestion).toHaveBeenCalledTimes(1))
  expect(
    requests.mock.calls.some(([url]) =>
      url.endsWith(
        '/diagnostic-sessions/session-real/current/audio?variant_task_id=v-main',
      ),
    ),
  ).toBe(true)
  expect(
    requests.mock.calls.some(([url]) =>
      url.endsWith('/task-audio/audio-main/file'),
    ),
  ).toBe(true)
})

it('reads the instruction with the browser voice while stored audio is being prepared', async () => {
  const requests = server()
  const spoken: string[] = []
  const speak = vi.fn((utterance: { text: string }) =>
    spoken.push(utterance.text),
  )
  vi.stubGlobal(
    'SpeechSynthesisUtterance',
    class {
      lang = ''
      voice = null
      onend: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor(readonly text: string) {}
    },
  )
  vi.stubGlobal('speechSynthesis', {
    speak,
    cancel: vi.fn(),
    getVoices: () => [],
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.includes('/current/audio?')
        ? Response.json({ variant_task_id: 'v-main', status: 'pending' })
        : requests(url, init),
    ),
  )
  vi.spyOn(audio, 'playQuestion')
  await home()
  await waitFor(() =>
    expect(spoken).toEqual(['Назовите модель и объясните решение.']),
  )
  expect(audio.playQuestion).not.toHaveBeenCalled()
  expect(screen.queryByText(/Аудио готовится/)).not.toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it('shows progress by topic and adds clarifying questions only when asked', async () => {
  const requests = server()
  const start = {
    ...progress,
    total_tasks: 6,
    competency_count: 2,
    current_competency: 1,
    current_step: 0,
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/diagnostic-sessions') && init?.method === 'POST')
        return Response.json(start, { status: 201 })
      if (url.endsWith('/answers'))
        return Response.json({
          ...start,
          current: basic,
          completed_tasks: 1,
          current_step: 1,
          text: 'Неуверенный ответ',
          score: 0,
          grader_score: 0,
          grader_max_score: 2,
          verdict: 'incorrect',
          criterion_results: criteria,
          feedback,
        })
      return requests(url, init)
    }),
  )
  await home()
  expect(await screen.findByLabelText('Тема 1: текущая')).toBeVisible()
  expect(screen.getByLabelText('Тема 2: ещё впереди')).toBeVisible()
  expect(screen.getByText('Тема 1 из 2', { selector: 'strong' })).toBeVisible()
  // The total number of questions is unknown, so no clarifying slots yet.
  expect(screen.queryByLabelText(/уточняющий вопрос/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  expect(await screen.findByLabelText('Тема 1: неверно')).toBeVisible()
  expect(
    screen.getByLabelText('Тема 1, уточняющий вопрос 1: текущий'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Следующее задание' }))
  expect(
    await screen.findByText('Тема 1 из 2 · уточняющий вопрос', {
      selector: 'strong',
    }),
  ).toBeVisible()
})

it('keeps navigator colors after a page reload', async () => {
  server()
  const first = await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  expect(await screen.findByLabelText('Вопрос 1: верно')).toBeVisible()
  first.unmount()
  await home()
  expect(await screen.findByLabelText('Вопрос 1: верно')).toBeVisible()
})

it('skips a question without sending audio for transcription', async () => {
  const requests = server()
  await home()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Пропустить вопрос' }),
  )
  expect(
    await screen.findByText(
      'Вопрос пропущен. Ответ оценён в 0 баллов. Продолжите со следующим вопросом.',
    ),
  ).toBeVisible()
  expect(screen.getByLabelText('Вопрос 1: пропущен')).toBeVisible()
  expect(audio.startRecording).not.toHaveBeenCalled()
  const [, init] = requests.mock.calls.find(([url]) => url.endsWith('/skip'))!
  expect(JSON.parse(init?.body as string)).toEqual({
    variant_task_id: 'v-main',
  })
  expect(requests.mock.calls.some(([url]) => url.endsWith('/answers'))).toBe(
    false,
  )
  expect(
    requests.mock.calls.some(([url]) => url.endsWith('/audio/skip-answer.wav')),
  ).toBe(false)
})

it('shows the crossed-out microphone when recording cannot start', async () => {
  server()
  vi.mocked(audio.startRecording).mockRejectedValueOnce(
    new DOMException('Denied', 'NotAllowedError'),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  expect(
    await screen.findByRole('heading', { name: 'Не удалось начать запись' }),
  ).toBeVisible()
  expect(
    screen.getByText('Проверьте доступ к микрофону и попробуйте ещё раз.'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Пропустить вопрос' }),
  ).toBeVisible()
})

it('keeps the manual button without an error when the browser blocks autoplay', async () => {
  server()
  vi.spyOn(audio, 'playQuestion')
    .mockRejectedValueOnce(new DOMException('Blocked', 'NotAllowedError'))
    .mockResolvedValue(vi.fn())
  await home()
  await waitFor(() => expect(audio.playQuestion).toHaveBeenCalledTimes(1))
  const replay = await screen.findByRole('button', {
    name: 'Прослушать инструкцию',
  })
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  fireEvent.click(replay)
  await waitFor(() => expect(audio.playQuestion).toHaveBeenCalledTimes(2))
})

it('allows starting a fresh diagnostic after the backend has lost the stored session', async () => {
  const requests = server()
  const first = await home()
  await screen.findByRole('heading', { name: 'Настоящий вопрос из банка' })
  first.unmount()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/diagnostic-sessions/session-real')
        ? Response.json(
            { code: 'DIAGNOSTIC_SESSION_NOT_FOUND' },
            { status: 404 },
          )
        : requests(url, init),
    ),
  )
  await home()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Сессия больше недоступна',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Начать заново' }))
  expect(
    await screen.findByRole('heading', { name: 'Настоящий вопрос из банка' }),
  ).toBeVisible()
  expect(
    requests.mock.calls.filter(
      ([url, init]) => url.endsWith('/variants') && init?.method === 'POST',
    ),
  ).toHaveLength(2)
})

it('uses backend basic tasks with a 0/1 scale and excludes them from the diagnostic total', async () => {
  const requests = server()
  let position = 0
  const tasks = [main, basic, basic2]
  const scores = [1, 1, 0]
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/answers')) {
        const index = position++
        return Response.json({
          ...progress,
          completed_tasks: position,
          skipped_tasks: 0,
          current: tasks[position],
          status: position === 3 ? 'completed' : 'active',
          score: scores[index],
          grader_score: scores[index],
          grader_max_score: index === 0 ? 2 : 1,
          verdict:
            index === 2 ? 'incorrect' : index === 1 ? 'correct' : 'partial',
          criterion_results: criteria,
          feedback,
        })
      }
      if (url.endsWith('/result'))
        return Response.json({
          ...resultBody,
          diagnostic_score: 1,
          completed_tasks: 3,
          untested_basics: [],
          answers: tasks.map((task, index) => ({
            ...resultBody.answers[0],
            task,
            variant_task_id: task.variant_task_id,
            source_task_id: task.source_task_id,
            role: task.role,
            score: scores[index],
            grader_score: scores[index],
            grader_max_score: index === 0 ? 2 : 1,
            verdict:
              index === 2 ? 'incorrect' : index === 1 ? 'correct' : 'partial',
          })),
        })
      return requests(url, init)
    }),
  )
  await home()
  for (let index = 0; index < 3; index++) {
    fireEvent.click(
      await screen.findByRole('button', { name: 'Начать запись' }),
    )
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      tasks[index].question,
    )
    if (index > 0)
      expect(
        screen.queryByRole('heading', { name: 'Варианты ответа' }),
      ).not.toBeInTheDocument()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Завершить запись' }),
    )
    expect(
      await screen.findByText(`${scores[index]} / ${index === 0 ? 2 : 1}`),
    ).toBeVisible()
    if (index === 1) expect(screen.getByText('Верно')).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', {
        name: index === 2 ? 'Посмотреть итог' : 'Следующее задание',
      }),
    )
  }
  expect(await screen.findByText('Диагностический балл')).toBeVisible()
  expect(
    screen.getByText('Диагностический балл').parentElement,
  ).toHaveTextContent('1 / 2')
  expect(screen.queryByText(/Не проверено/)).not.toBeInTheDocument()
})

it('explains when no competency in the map is eligible instead of displaying mock tasks', async () => {
  const requests = server()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/variants')
        ? Response.json(
            {
              code: 'NO_ELIGIBLE_COMPETENCIES',
              message: 'Нет подходящих компетенций',
            },
            { status: 409 },
          )
        : requests(url, init),
    ),
  )
  await home()
  expect(await screen.findByRole('alert')).toHaveTextContent('карта')
  expect(
    screen.queryByRole('button', { name: 'Начать запись' }),
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Классификация')).not.toBeInTheDocument()
})

it('returns to login if diagnostic submission loses authorization', async () => {
  const requests = server()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/answers')
        ? Response.json({ code: 'UNAUTHORIZED' }, { status: 401 })
        : requests(url, init),
    ),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Как вы хотите войти?' }),
  ).toBeVisible()
  expect(
    screen.queryByText('Настоящий вопрос из банка'),
  ).not.toBeInTheDocument()
})

it('retries variant and session creation with the same keys after lost responses', async () => {
  const requests = server()
  const variantKeys: unknown[] = []
  const sessionKeys: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/variants')) {
        variantKeys.push(init?.headers)
        if (variantKeys.length === 1) throw new TypeError('offline')
      }
      if (url.endsWith('/diagnostic-sessions') && init?.method === 'POST') {
        sessionKeys.push(init.headers)
        if (sessionKeys.length === 1) throw new TypeError('offline')
      }
      return requests(url, init)
    }),
  )
  await home()
  await screen.findByRole('alert')
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  await waitFor(() => expect(sessionKeys).toHaveLength(1))
  await screen.findByRole('alert')
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(
    await screen.findByRole('heading', { name: 'Настоящий вопрос из банка' }),
  ).toBeVisible()
  expect(variantKeys).toHaveLength(2)
  expect(variantKeys[0]).toEqual(variantKeys[1])
  expect(sessionKeys).toHaveLength(2)
  expect(sessionKeys[0]).toEqual(sessionKeys[1])
})

it('retains the original audio/key when the backend reports that an answer is still in progress', async () => {
  const requests = server()
  let attempt = 0
  const submissions: RequestInit[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/answers')) {
        submissions.push(init!)
        if (++attempt === 1)
          return Response.json(
            { code: 'DIAGNOSTIC_ANSWER_IN_PROGRESS' },
            { status: 409 },
          )
      }
      return requests(url, init)
    }),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  await screen.findByRole('alert')
  fireEvent.click(screen.getByRole('button', { name: 'Отправить снова' }))
  expect(await screen.findByText(/Серверное объяснение\./)).toBeVisible()
  expect(submissions[0].headers).toEqual(submissions[1].headers)
  expect(submissions[0].body).toBe(submissions[1].body)
})

it('offers only a new recording when speech was not recognized', async () => {
  const requests = server()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/answers')
        ? Response.json(
            {
              code: 'NO_SPEECH_DETECTED',
              message: 'Речь не распознана; повторите запись.',
            },
            { status: 422 },
          )
        : requests(url, init),
    ),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Речь не распознана',
  )
  expect(
    screen.queryByRole('button', { name: 'Отправить снова' }),
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Попробуйте ещё раз')).not.toBeInTheDocument()
  expect(
    screen.getAllByRole('button', { name: 'Записать заново' }),
  ).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Записать заново' }))
  expect(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  ).toBeVisible()
})

it('offers a new diagnostic if the completed result disappears after a backend restart', async () => {
  const requests = server()
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/result')
        ? Response.json(
            { code: 'DIAGNOSTIC_SESSION_NOT_FOUND' },
            { status: 404 },
          )
        : requests(url, init),
    ),
  )
  await home()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Сессия больше недоступна',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Начать заново' }))
  expect(
    await screen.findByRole('button', { name: 'Начать запись' }),
  ).toBeVisible()
})

it.each([1, 2])(
  'finishes a competency with %i available tasks without inventing missing basics',
  async (totalTasks) => {
    server(200, totalTasks)
    await home()
    expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
      'Настоящий вопрос из банка',
    )
    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-valuemax',
      String(totalTasks),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Начать запись' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Завершить запись' }),
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Посмотреть итог' }),
    )
    expect(
      await screen.findByRole('heading', { name: 'Сессия завершена' }),
    ).toBeVisible()
    expect(
      screen.getByText('Диагностический балл').parentElement,
    ).toHaveTextContent('2 / 2')
    fireEvent.click(
      await screen.findByRole('button', { name: /Разбор по заданиям/ }),
    )
    expect(screen.queryByText('Второй базовый вопрос')).not.toBeInTheDocument()
    if (totalTasks === 1)
      expect(
        screen.queryByText('Базовый вопрос из банка'),
      ).not.toBeInTheDocument()
    else expect(screen.getByText('Базовый вопрос из банка')).toBeVisible()
  },
)
