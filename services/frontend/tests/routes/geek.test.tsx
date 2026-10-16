import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { focusManager } from '@tanstack/react-query'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'
import { validWavBlob } from '../support/audio'

type ApiTurn = {
  id: string
  line: number
  speaker: 'geek' | 'student'
  kind: string
  node_id: string | null
  source: string | null
  text: string
  created_at: number
}

const geekTurn = (
  id: string,
  line: number,
  kind: string,
  text: string,
): ApiTurn => ({
  id,
  line,
  speaker: 'geek',
  kind,
  node_id: kind === 'closing' ? null : 'a-star',
  source: null,
  text,
  created_at: line,
})

// A contract-shaped backend: every answer appends the student turn and the
// next scripted Geek turn; the third answer completes the diagnostic.
function geekBackend(
  options: {
    graphReady?: boolean
    active?: boolean
    failAnswers?: number
  } = {},
) {
  const script = [
    geekTurn(
      't1',
      1,
      'question',
      'Что изменится в A*, если эвристика переоценивает путь?',
    ),
    geekTurn('t3', 3, 'followup', 'А где допустимая эвристика не поможет?'),
    geekTurn('t5', 5, 'question', 'Что такое **эвристическая функция**?'),
    geekTurn(
      't7',
      7,
      'closing',
      'Спасибо! Диагностика завершена, итог уже готов.',
    ),
  ]
  let failures = options.failAnswers ?? 0
  const control = { failReads: 0, failStarts: 0 }
  let session: {
    id: string
    subject_id: string
    status: string
    progress_percent: number
    current_turn_id: string | null
    started_at: number
    completed_at: number | null
    turns: ApiTurn[]
  } | null = options.active
    ? {
        id: 'd1',
        subject_id: 'subject-1',
        status: 'active',
        progress_percent: 0,
        current_turn_id: 't1',
        started_at: 1791158400,
        completed_at: null,
        turns: [script[0]],
      }
    : null
  const answers: { key: string; body: Record<string, unknown> }[] = []
  type ApiSession = NonNullable<typeof session>
  function accept(
    current: ApiSession,
    body: Record<string, unknown>,
  ): ApiSession {
    let session = current
    const n = session.turns.length
    const student: ApiTurn = {
      id: `t${n + 1}`,
      line: n + 1,
      speaker: 'student',
      kind: 'answer',
      node_id: 'a-star',
      source: body.dont_know ? 'dont_know' : body.text ? 'text' : 'voice',
      text: body.dont_know ? 'Не знаю.' : String(body.text ?? 'Ответ голосом'),
      created_at: n + 1,
    }
    const next = script[(n + 1) / 2]
    const done = next.kind === 'closing'
    session = {
      ...session,
      status: done ? 'completed' : 'active',
      progress_percent: done ? 100 : session.progress_percent + 30,
      current_turn_id: done ? null : next.id,
      completed_at: done ? 1791160000 : null,
      turns: [...session.turns, student, next],
    }
    return session
  }
  // An answer accepted elsewhere, for example in another tab.
  const advance = () => {
    if (session) session = accept(session, { text: 'Ответ из другой вкладки' })
  }

  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    if (url.endsWith('/auth/me'))
      return Response.json({
        id: 'student-1',
        email: 'student@example.com',
        display_name: 'Студент',
        role: 'student',
      })
    if (url.endsWith('/subjects'))
      return Response.json([
        { id: 'subject-1', name: 'Введение в ИИ', ready: true },
      ])
    if (url.includes('/diagnostic-sessions?subject_id=subject-1'))
      return Response.json({
        items: session
          ? [
              {
                id: session.id,
                status: session.status,
                progress_percent: session.progress_percent,
                started_at: session.started_at,
                completed_at: session.completed_at,
                grade: session.status === 'completed' ? 'B' : null,
                score_percent: session.status === 'completed' ? 81 : null,
              },
            ]
          : [],
      })
    if (url.endsWith('/diagnostic-sessions') && method === 'POST') {
      if (control.failStarts > 0) {
        control.failStarts--
        return Response.json(
          { code: 'PROCESSING_UNAVAILABLE', message: 'Недоступно.' },
          { status: 503 },
        )
      }
      if (options.graphReady === false)
        return Response.json(
          { code: 'GRAPH_NOT_READY', message: 'Граф курса ещё не собран.' },
          { status: 409 },
        )
      session ??= {
        id: 'd1',
        subject_id: 'subject-1',
        status: 'active',
        progress_percent: 0,
        current_turn_id: 't1',
        started_at: 1791158400,
        completed_at: null,
        turns: [script[0]],
      }
      return Response.json(session, { status: 201 })
    }
    if (url.endsWith('/diagnostic-sessions/d1') && session) {
      if (control.failReads > 0) {
        control.failReads--
        return Response.json(
          { code: 'PROCESSING_UNAVAILABLE', message: 'Недоступно.' },
          { status: 503 },
        )
      }
      return Response.json(session)
    }
    if (url.endsWith('/diagnostic-sessions/d1/answers') && session) {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>
      answers.push({
        key: new Headers(init?.headers).get('Idempotency-Key') ?? '',
        body,
      })
      if (failures > 0) {
        failures--
        return Response.json(
          { code: 'MODEL_UNAVAILABLE', message: 'Модель недоступна.' },
          { status: 503 },
        )
      }
      session = accept(session, body)
      return Response.json(session)
    }
    if (url.endsWith('/diagnostic-sessions/d1/result'))
      return Response.json({
        session_id: 'd1',
        grade: 'B',
        score_percent: 81,
        nodes: [
          {
            node_id: 'search-space',
            title: 'Пространство состояний',
            status: 'credited_by_parent',
            grade: null,
            top_level: false,
          },
          {
            node_id: 'heuristic',
            title: 'Эвристическая функция',
            status: 'gap',
            grade: 'D',
            top_level: false,
          },
          {
            node_id: 'a-star',
            title: 'Алгоритм A*',
            status: 'assisted',
            grade: 'B',
            top_level: true,
          },
        ],
        gaps: [{ node_id: 'heuristic', title: 'Эвристическая функция' }],
      })
    if (/\/diagnostic-sessions\/d1\/turns\/t\d+\/audio$/.test(url))
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    if (url.endsWith('/voice/transcriptions') && method === 'POST')
      return Response.json({
        id: 'tr-1',
        text: 'Эвристика оценивает путь до цели',
        created_at: 1,
      })
    throw new Error(`Unexpected request: ${method} ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  return { fetch, answers, control, advance }
}

class FakeAudio {
  static played: string[] = []
  static paused = 0
  onended: (() => void) | null = null
  onerror: (() => void) | null = null
  error = null
  constructor(readonly src: string) {}
  play() {
    FakeAudio.played.push(this.src)
    return Promise.resolve()
  }
  pause() {
    FakeAudio.paused++
  }
  removeAttribute() {}
}

beforeEach(() => {
  sessionStorage.clear()
  FakeAudio.played = []
  FakeAudio.paused = 0
  vi.stubGlobal('Audio', FakeAudio)
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:turn')
  vi.spyOn(URL, 'revokeObjectURL').mockReturnValue(undefined)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function renderApp() {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
}

async function start() {
  const button = await screen.findByRole('button', {
    name: 'Начать диагностику',
  })
  await waitFor(() => expect(button).toBeEnabled())
  fireEvent.click(button)
}

const answerField = () => screen.findByRole('textbox', { name: 'Ваш ответ' })

async function answerByText(text: string) {
  const field = await answerField()
  fireEvent.change(field, { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ответ' }))
}

it('runs a diagnostic from the home page to the result and back to the history', async () => {
  const { answers, fetch } = geekBackend()
  renderApp()
  expect(
    await screen.findByText('Вы ещё не проходили диагностику по этому курсу.'),
  ).toBeVisible()
  await start()

  expect(
    await screen.findByText(
      'Что изменится в A*, если эвристика переоценивает путь?',
    ),
  ).toBeVisible()
  expect(
    screen.getByRole('progressbar', { name: 'Диагностика' }),
  ).toHaveAttribute('aria-valuenow', '0')
  // The new Geek turn is voiced at once and can be stopped.
  await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
  expect(
    await screen.findByRole('button', { name: 'Остановить озвучку' }),
  ).toBeVisible()
  expect(
    fetch.mock.calls.some(([url]) => url.endsWith('/turns/t1/audio')),
  ).toBe(true)

  await answerByText('Перестанет быть оптимальным')
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(screen.getByText('Перестанет быть оптимальным')).toBeVisible()
  expect(screen.getByText('Дополнительный вопрос')).toBeVisible()
  expect(answers[0].body).toEqual({
    turn_id: 't1',
    text: 'Перестанет быть оптимальным',
  })
  expect(answers[0].key).toMatch(/^answer:/)

  fireEvent.click(screen.getByRole('button', { name: 'Не знаю' }))
  expect(await screen.findByText('эвристическая функция')).toBeVisible()
  expect(answers[1].body).toEqual({ turn_id: 't3', dont_know: true })

  await answerByText('Оценка расстояния до цели')
  expect(await screen.findByText('Диагностика завершена.')).toBeVisible()
  expect(
    screen.queryByRole('textbox', { name: 'Ваш ответ' }),
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('progressbar', { name: 'Диагностика' }),
  ).toHaveAttribute('aria-valuenow', '100')

  fireEvent.click(screen.getByRole('button', { name: 'Перейти к итогу' }))
  expect(
    await screen.findByRole('heading', { name: 'Итог диагностики', level: 1 }),
  ).toBeVisible()
  const grade = await screen.findByRole('region', {
    name: 'Предварительная оценка',
  })
  expect(within(grade).getByText('B')).toBeVisible()
  expect(screen.getByText('81% от максимума')).toBeVisible()
  const gaps = screen.getByRole('region', { name: 'Что подтянуть' })
  expect(within(gaps).getByText('Эвристическая функция')).toBeVisible()
  const topics = screen.getByRole('region', { name: 'Все темы' })
  expect(within(topics).getByText('С помощью')).toBeVisible()
  expect(within(topics).getByText('Засчитано по основной теме')).toBeVisible()

  fireEvent.click(screen.getAllByRole('button', { name: 'На главную' })[0])
  expect(await screen.findByText('Оценка B · 81%')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Открыть итог' }))
  expect(await screen.findByText('81% от максимума')).toBeVisible()
})

it('resumes the active diagnostic from the history', async () => {
  geekBackend({ active: true })
  renderApp()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить диагностику' }),
  )
  expect(
    await screen.findByText(
      'Что изменится в A*, если эвристика переоценивает путь?',
    ),
  ).toBeVisible()
  expect(await answerField()).toBeEnabled()
})

it('keeps the answer after a model failure and resends it with the same key', async () => {
  const { answers } = geekBackend({ failAnswers: 1 })
  renderApp()
  await start()
  await answerByText('Перестанет быть оптимальным')
  expect(await screen.findByText('Geek сейчас не отвечает.')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ещё раз' }))
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(answers).toHaveLength(2)
  expect(answers[1].key).toBe(answers[0].key)
})

it('says the course is not ready when its graph is not built', async () => {
  geekBackend({ graphReady: false })
  renderApp()
  await start()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'граф тем не собран',
  )
})

it('answers by voice after the student checks the transcript', async () => {
  const { answers } = geekBackend()
  let recorder:
    | {
        onstop: (() => void) | null
        ondataavailable: ((event: { data: Blob }) => void) | null
      }
    | undefined
  class FakeRecorder {
    static isTypeSupported() {
      return true
    }
    state = 'inactive'
    mimeType = 'audio/webm'
    ondataavailable: ((event: { data: Blob }) => void) | null = null
    onstop: (() => void) | null = null
    constructor() {
      // eslint-disable-next-line @typescript-eslint/no-this-alias -- the test drives the recorder it creates.
      recorder = this
    }
    start() {
      this.state = 'recording'
    }
    stop() {
      this.state = 'inactive'
      this.ondataavailable?.({
        data: new Blob(['voice'], { type: 'audio/webm' }),
      })
      this.onstop?.()
    }
  }
  vi.stubGlobal('MediaRecorder', FakeRecorder)
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: {
      getUserMedia: () => Promise.resolve({ getTracks: () => [{ stop() {} }] }),
    },
  })
  renderApp()
  await start()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Ответить голосом' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Остановить запись' }),
  )
  expect(recorder).toBeDefined()
  const transcript = await screen.findByRole('region', {
    name: 'Проверьте расшифровку',
  })
  expect(
    within(transcript).getByText('Эвристика оценивает путь до цели'),
  ).toBeVisible()
  await act(async () => {
    fireEvent.click(
      within(transcript).getByRole('button', { name: 'Отправить' }),
    )
  })
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(answers[0].body).toEqual({ turn_id: 't1', transcription_id: 'tr-1' })
})

// A tab regains focus: React Query refetches the open session.
async function refocus() {
  await act(async () => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
}

it('retries the original turn and drops the retry once the session moved on', async () => {
  const { answers, advance } = geekBackend({ failAnswers: 2 })
  renderApp()
  await start()
  await answerByText('Перестанет быть оптимальным')
  expect(await screen.findByText('Geek сейчас не отвечает.')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ещё раз' }))
  expect(await screen.findByText('Geek сейчас не отвечает.')).toBeVisible()
  expect(answers[1].body).toEqual(answers[0].body)
  expect(answers[1].key).toBe(answers[0].key)
  // The answer was accepted in another tab; the refreshed dialogue shows the
  // next question and no longer offers to resend the old answer.
  advance()
  await refocus()
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Отправить ещё раз' }),
  ).not.toBeInTheDocument()
  expect(answers).toHaveLength(2)
})

it('keeps an unsent draft when a background refresh fails', async () => {
  const { control } = geekBackend()
  renderApp()
  await start()
  const field = await answerField()
  fireEvent.change(field, { target: { value: 'Черновик ответа' } })
  control.failReads = 1
  await refocus()
  const alert = await screen.findByText(
    'Не удалось обновить диагностику. Ваш ответ сохранён в поле.',
  )
  expect(alert).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Ваш ответ' })).toHaveValue(
    'Черновик ответа',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  await waitFor(() =>
    expect(
      screen.queryByText(
        'Не удалось обновить диагностику. Ваш ответ сохранён в поле.',
      ),
    ).not.toBeInTheDocument(),
  )
  expect(screen.getByRole('textbox', { name: 'Ваш ответ' })).toHaveValue(
    'Черновик ответа',
  )
})

it('stops Geek audio before opening the microphone and keeps it silent while recording', async () => {
  geekBackend()
  class FakeRecorder {
    static isTypeSupported() {
      return true
    }
    state = 'inactive'
    mimeType = 'audio/webm'
    ondataavailable: ((event: { data: Blob }) => void) | null = null
    onstop: (() => void) | null = null
    start() {
      this.state = 'recording'
    }
    stop() {
      this.state = 'inactive'
      this.onstop?.()
    }
  }
  vi.stubGlobal('MediaRecorder', FakeRecorder)
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: {
      getUserMedia: () => Promise.resolve({ getTracks: () => [{ stop() {} }] }),
    },
  })
  renderApp()
  await start()
  expect(
    await screen.findByRole('button', { name: 'Остановить озвучку' }),
  ).toBeVisible()
  const pausedBefore = FakeAudio.paused
  fireEvent.click(screen.getByRole('button', { name: 'Ответить голосом' }))
  expect(
    await screen.findByRole('button', { name: 'Остановить запись' }),
  ).toBeVisible()
  expect(FakeAudio.paused).toBeGreaterThan(pausedBefore)
  expect(
    screen.getByRole('button', { name: 'Прослушать реплику' }),
  ).toBeDisabled()
})

it('reports a failed start again on the result screen', async () => {
  const { control } = geekBackend()
  renderApp()
  await start()
  await answerByText('Перестанет быть оптимальным')
  fireEvent.click(await screen.findByRole('button', { name: 'Не знаю' }))
  await screen.findByText('эвристическая функция')
  await answerByText('Оценка расстояния до цели')
  fireEvent.click(
    await screen.findByRole('button', { name: 'Перейти к итогу' }),
  )
  await screen.findByText('81% от максимума')
  control.failStarts = 1
  fireEvent.click(screen.getByRole('button', { name: 'Пройти заново' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось начать диагностику.',
  )
  expect(screen.getByRole('button', { name: 'Пройти заново' })).toBeEnabled()
})

it('gives the dialogue a level-one heading', async () => {
  geekBackend()
  renderApp()
  await start()
  expect(
    await screen.findByRole('heading', { name: 'Устный экзамен', level: 1 }),
  ).toBeInTheDocument()
})

it.each(['mediaDevices', 'MediaRecorder', 'permission'])(
  'restores Geek playback when microphone start fails: %s',
  async (failure) => {
    vi.stubGlobal(
      'MediaRecorder',
      failure === 'MediaRecorder' ? undefined : class {},
    )
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value:
        failure === 'mediaDevices'
          ? undefined
          : {
              getUserMedia: () =>
                Promise.reject(new DOMException('Denied', 'NotAllowedError')),
            },
    })
    geekBackend()
    renderApp()
    await start()
    await screen.findByRole('button', { name: 'Остановить озвучку' })
    const pausedBefore = FakeAudio.paused
    fireEvent.click(screen.getByRole('button', { name: 'Ответить голосом' }))
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(FakeAudio.paused).toBeGreaterThan(pausedBefore)
    const listen = screen.getByRole('button', { name: 'Прослушать реплику' })
    await waitFor(() => expect(listen).toBeEnabled())
    fireEvent.click(listen)
    expect(
      await screen.findByRole('button', { name: 'Остановить озвучку' }),
    ).toBeEnabled()
  },
)
