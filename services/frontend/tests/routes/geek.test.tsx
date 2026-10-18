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
  reply?: string
  created_at: number
}

const geekTurn = (
  id: string,
  line: number,
  kind: string,
  text: string,
  reply = '',
): ApiTurn => ({
  id,
  line,
  speaker: 'geek',
  kind,
  node_id: kind === 'closing' ? null : 'a-star',
  source: null,
  text: reply ? `${reply}\n\n${text}` : text,
  reply,
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
    geekTurn(
      't3',
      3,
      'followup',
      'А где допустимая эвристика не поможет?',
      'Верно про оптимальность.',
    ),
    geekTurn('t5', 5, 'question', 'Что такое **эвристическая функция**?'),
    geekTurn(
      't7',
      7,
      'closing',
      'Спасибо! Диагностика завершена, итог уже готов.',
    ),
  ]
  let failures = options.failAnswers ?? 0
  // cancelled: the course graph was rebuilt, so answers find no active turn.
  // readGate and answerGate hold the responses until they resolve; a held
  // read returns the session as it was when the request arrived.
  const control = {
    failReads: 0,
    failStarts: 0,
    cancelled: false,
    readGate: null as Promise<void> | null,
    answerGate: null as Promise<void> | null,
    // Holds the response of an answer the backend has already accepted.
    replyGate: null as Promise<void> | null,
    // Holds Geek's voice; failVoice answers it with 503.
    voiceGate: null as Promise<void> | null,
    failVoice: false,
  }
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
    if (control.cancelled && session?.status === 'active')
      session = { ...session, status: 'cancelled', current_turn_id: null }
    if (url.endsWith('/diagnostic-sessions/d1') && session) {
      if (control.failReads > 0) {
        control.failReads--
        return Response.json(
          { code: 'PROCESSING_UNAVAILABLE', message: 'Недоступно.' },
          { status: 503 },
        )
      }
      const snapshot = session
      if (control.readGate) await control.readGate
      return Response.json(snapshot)
    }
    if (url.endsWith('/diagnostic-sessions/d1/answers') && session) {
      if (control.answerGate) await control.answerGate
      if (session.status === 'cancelled')
        return Response.json(
          { code: 'SESSION_NOT_ACTIVE', message: 'Диагностика уже завершена.' },
          { status: 409 },
        )
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
      const accepted = session
      if (control.replyGate) await control.replyGate
      return Response.json(accepted)
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
            requires: ['search-space'],
          },
          {
            node_id: 'a-star',
            title: 'Алгоритм A*',
            status: 'assisted',
            grade: 'B',
            top_level: true,
            requires: ['heuristic', 'annealing'],
          },
          {
            node_id: 'annealing',
            title: 'Имитация отжига',
            status: 'unchecked',
            grade: null,
            top_level: true,
          },
        ],
        gaps: [{ node_id: 'heuristic', title: 'Эвристическая функция' }],
      })
    if (
      /\/diagnostic-sessions\/d1\/turns\/t\d+\/audio\?part=(reply|question)$/.test(
        url,
      )
    ) {
      if (control.voiceGate)
        await new Promise<void>((resolve, reject) => {
          // Like a browser, an aborted request rejects at once.
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('Aborted', 'AbortError')),
          )
          void control.voiceGate!.then(resolve)
        })
      if (control.failVoice)
        return Response.json(
          { code: 'PROCESSING_UNAVAILABLE', message: 'Недоступно.' },
          { status: 503 },
        )
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    }
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
  static last: FakeAudio | null = null
  onended: (() => void) | null = null
  onerror: (() => void) | null = null
  error = null
  constructor(readonly src: string) {
    FakeAudio.last = this
  }
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
  FakeAudio.last = null
  vi.stubGlobal('Audio', FakeAudio)
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:turn')
  vi.spyOn(URL, 'revokeObjectURL').mockReturnValue(undefined)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function renderApp() {
  const queryClient = createQueryClient()
  const router = createRouter({
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  return queryClient
}

function gate() {
  let open = () => {}
  const closed = new Promise<void>((resolve) => (open = resolve))
  return { closed, open }
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

// Answers the last question and lets Geek's closing voice play to its end,
// as a browser does; the result card waits for it.
async function finishDiagnostic(text: string) {
  const played = FakeAudio.played.length
  await answerByText(text)
  await waitFor(() => expect(FakeAudio.played).toHaveLength(played + 1))
  act(() => FakeAudio.last!.onended!())
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
    fetch.mock.calls.some(([url]) =>
      url.endsWith('/turns/t1/audio?part=question'),
    ),
  ).toBe(true)
  // The first question replies to nothing, so it has no reply part.
  expect(
    fetch.mock.calls.some(([url]) =>
      url.endsWith('/turns/t1/audio?part=reply'),
    ),
  ).toBe(false)

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

  await answerByText('Эвристическая функция')
  expect(await screen.findByText('эвристическая функция')).toBeVisible()
  expect(answers[1].body).toEqual({
    turn_id: 't3',
    text: 'Эвристическая функция',
  })

  await finishDiagnostic('Оценка расстояния до цели')
  expect(
    await screen.findByRole('heading', {
      name: 'Диагностика завершена',
      level: 2,
    }),
  ).toBeVisible()
  expect(screen.getByText('Geek разобрал 1 тему — итог готов')).toBeVisible()
  expect(
    screen.queryByRole('textbox', { name: 'Ваш ответ' }),
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('progressbar', { name: 'Диагностика' }),
  ).toHaveAttribute('aria-valuenow', '100')

  const toResult = screen.getByRole('button', { name: 'Посмотреть итог' })
  expect(toResult).toHaveFocus()
  fireEvent.click(toResult)
  expect(
    await screen.findByRole('heading', { name: 'Итог диагностики', level: 1 }),
  ).toBeVisible()
  const grade = await screen.findByRole('region', {
    name: 'Предварительная оценка',
  })
  expect(within(grade).getByText('B')).toBeVisible()
  expect(screen.getByText('81% от максимума')).toBeVisible()
  // The summary counts topics by status and skips statuses with no topics.
  const summary = within(grade).getByRole('list', { name: 'Темы по статусам' })
  expect(
    within(summary)
      .getAllByRole('listitem')
      .map((item) => item.textContent),
  ).toEqual(['1Засчитано по основной теме', '1С помощью', '1Пробел'])
  const gaps = screen.getByRole('region', { name: 'Что подтянуть' })
  expect(within(gaps).getByText('Эвристическая функция')).toBeVisible()
  const topics = screen.getByRole('region', { name: 'Все темы' })
  expect(within(topics).getByText('С помощью')).toBeVisible()
  expect(within(topics).getByText('Засчитано по основной теме')).toBeVisible()
  // Topics the diagnostic never reached are not part of the feedback.
  expect(within(topics).queryByText('Имитация отжига')).not.toBeInTheDocument()
  expect(screen.queryByText('Не проверялась')).not.toBeInTheDocument()
  // The map shows the checked subgraph and says what each topic relies on.
  const map = screen.getByRole('list', { name: 'Темы на карте' })
  expect(
    within(map)
      .getAllByRole('listitem')
      .map((item) => item.firstChild?.textContent),
  ).toEqual(['Пространство состояний', 'Эвристическая функция', 'Алгоритм A*'])
  expect(
    within(map).getByText('Опирается на: Эвристическая функция'),
  ).toBeInTheDocument()
  expect(
    within(map).getByText('Опирается на: Пространство состояний'),
  ).toBeInTheDocument()

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

it('sends a spoken answer as soon as it is transcribed', async () => {
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
  // The transcript is sent at once: no check or re-recording step.
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Записать заново' })).toBeNull()
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
  await answerByText('Эвристическая функция')
  await screen.findByText('эвристическая функция')
  await finishDiagnostic('Оценка расстояния до цели')
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  await screen.findByText('81% от максимума')
  control.failStarts = 1
  fireEvent.click(screen.getByRole('button', { name: 'Пройти заново' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось начать диагностику.',
  )
  expect(screen.getByRole('button', { name: 'Пройти заново' })).toBeEnabled()
})

it('offers the result only after Geek has voiced the closing reply', async () => {
  geekBackend()
  renderApp()
  await start()
  await answerByText('Перестанет быть оптимальным')
  await answerByText('Эвристическая функция')
  await screen.findByText('эвристическая функция')
  const played = FakeAudio.played.length
  await answerByText('Оценка расстояния до цели')
  expect(
    await screen.findByText('Спасибо! Диагностика завершена, итог уже готов.'),
  ).toBeVisible()
  await waitFor(() => expect(FakeAudio.played).toHaveLength(played + 1))
  // The closing reply is still being voiced: the result waits for it.
  expect(
    screen.queryByRole('button', { name: 'Посмотреть итог' }),
  ).not.toBeInTheDocument()
  act(() => FakeAudio.last!.onended!())
  expect(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  ).toBeVisible()
})

it('offers the result when the closing reply cannot be voiced', async () => {
  const { control } = geekBackend()
  renderApp()
  await start()
  await answerByText('Перестанет быть оптимальным')
  await answerByText('Эвристическая функция')
  await screen.findByText('эвристическая функция')
  control.failVoice = true
  await answerByText('Оценка расстояния до цели')
  expect(
    await screen.findByText('Спасибо! Диагностика завершена, итог уже готов.'),
  ).toBeVisible()
  expect(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  ).toBeVisible()
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

it('voices the reply to the answer first and the question after it in one turn', async () => {
  const { fetch } = geekBackend()
  renderApp()
  await start()
  await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
  await answerByText('Перестанет быть оптимальным')
  // One Geek message holds both parts.
  expect(await screen.findByText('Верно про оптимальность.')).toBeVisible()
  expect(
    screen.getByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  const audioUrls = () =>
    fetch.mock.calls
      .map(([url]) => url)
      .filter((url) => url.includes('/turns/t3/audio'))
  await waitFor(() => expect(FakeAudio.played).toHaveLength(2))
  await waitFor(() =>
    expect(audioUrls()).toEqual([
      '/api/v1/diagnostic-sessions/d1/turns/t3/audio?part=reply',
      '/api/v1/diagnostic-sessions/d1/turns/t3/audio?part=question',
    ]),
  )
  // The question plays once the reply has ended.
  act(() => FakeAudio.last?.onended?.())
  await waitFor(() => expect(FakeAudio.played).toHaveLength(3))
})

it('pops up that the course was updated when the diagnostic is cancelled', async () => {
  const { control } = geekBackend({ active: true })
  renderApp()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить диагностику' }),
  )
  await answerField()
  control.cancelled = true
  await answerByText('Перестанет быть оптимальным')
  expect(
    await screen.findByText('Курс обновлён: диагностику нужно начать заново.'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Закрыть уведомление' }))
  await waitFor(() =>
    expect(
      screen.queryByText('Курс обновлён: диагностику нужно начать заново.'),
    ).not.toBeInTheDocument(),
  )
  expect(
    screen.getByText(
      'Курс обновился, и эта диагностика прервана. Начните новую на главной.',
    ),
  ).toBeVisible()
})

it('keeps the accepted answer when a background read started before it lands late', async () => {
  const { control, fetch } = geekBackend()
  const queryClient = renderApp()
  await start()
  await answerField()
  // The answer is being evaluated when the periodic read of the session
  // goes out; the read sees the old turn and comes back after the answer.
  const answer = gate()
  control.answerGate = answer.closed
  await answerByText('Перестанет быть оптимальным')
  await waitFor(() =>
    expect(fetch.mock.calls.some(([url]) => url.endsWith('/d1/answers'))).toBe(
      true,
    ),
  )
  const read = gate()
  control.readGate = read.closed
  const refetch = queryClient.refetchQueries({
    queryKey: ['diagnostic-session'],
  })
  answer.open()
  expect(
    await screen.findByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  read.open()
  await act(() => refetch)
  const [[, cached]] = queryClient.getQueriesData<{ currentTurnId: string }>({
    queryKey: ['diagnostic-session'],
  })
  expect(cached?.currentTurnId).toBe('t3')
  expect(
    screen.getByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(screen.getByText('Перестанет быть оптимальным')).toBeVisible()
})

it('keeps a diagnostic cancelled when the response of an earlier answer lands late', async () => {
  const { control, fetch } = geekBackend()
  const queryClient = renderApp()
  await start()
  await answerField()
  // The backend accepts the answer, but its response is slow; meanwhile the
  // graph is rebuilt and the periodic read sees the diagnostic cancelled.
  const reply = gate()
  control.replyGate = reply.closed
  await answerByText('Перестанет быть оптимальным')
  await waitFor(() =>
    expect(fetch.mock.calls.some(([url]) => url.endsWith('/d1/answers'))).toBe(
      true,
    ),
  )
  control.cancelled = true
  await act(() =>
    queryClient.refetchQueries({ queryKey: ['diagnostic-session'] }),
  )
  const status = () =>
    queryClient.getQueriesData<{ status: string }>({
      queryKey: ['diagnostic-session'],
    })[0][1]?.status
  expect(status()).toBe('cancelled')
  reply.open()
  await waitFor(() =>
    expect(
      queryClient.isMutating({ mutationKey: ['diagnostic', 'answer'] }),
    ).toBe(0),
  )
  expect(status()).toBe('cancelled')
  expect(
    screen.getByText(
      'Курс обновился, и эта диагностика прервана. Начните новую на главной.',
    ),
  ).toBeVisible()
})

it('keeps the reply hidden behind the thinking animation until its voice starts', async () => {
  const { control, fetch } = geekBackend()
  renderApp()
  await start()
  await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
  const voice = gate()
  control.voiceGate = voice.closed
  await answerByText('Перестанет быть оптимальным')
  // The answer is accepted, but the reaction is not voiced yet.
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(([url]) => url.endsWith('/t3/audio?part=reply')),
    ).toBe(true),
  )
  expect(screen.getByText('Geek думает…')).toBeVisible()
  expect(screen.queryByText('Верно про оптимальность.')).toBeNull()
  expect(screen.getByRole('textbox', { name: 'Ваш ответ' })).toBeDisabled()

  voice.open()
  expect(await screen.findByText('Верно про оптимальность.')).toBeVisible()
  expect(
    screen.getByText('А где допустимая эвристика не поможет?'),
  ).toBeVisible()
  expect(screen.queryByText('Geek думает…')).toBeNull()
  expect(screen.getByRole('textbox', { name: 'Ваш ответ' })).toBeEnabled()
})

it('shows the reply when its voice fails instead of thinking forever', async () => {
  const { control } = geekBackend()
  renderApp()
  await start()
  await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
  control.failVoice = true
  await answerByText('Перестанет быть оптимальным')
  expect(await screen.findByText('Верно про оптимальность.')).toBeVisible()
  expect(screen.queryByText('Geek думает…')).toBeNull()
})

it('stops a voice that comes after the reply was shown without it', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const { control, fetch } = geekBackend()
    renderApp()
    await start()
    await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
    const voice = gate()
    control.voiceGate = voice.closed
    await answerByText('Перестанет быть оптимальным')
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(([url]) => url.endsWith('/t3/audio?part=reply')),
      ).toBe(true),
    )
    await act(() => vi.advanceTimersByTimeAsync(20_000))
    expect(screen.getByText('Верно про оптимальность.')).toBeVisible()
    expect(screen.getByRole('textbox', { name: 'Ваш ответ' })).toBeEnabled()

    voice.open()
    await act(() => vi.advanceTimersByTimeAsync(100))
    expect(FakeAudio.played).toHaveLength(1)
  } finally {
    vi.useRealTimers()
  }
})

it('does not let a stopped voice of an older reply show the next one', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const { control, fetch } = geekBackend()
    renderApp()
    await start()
    await waitFor(() => expect(FakeAudio.played).toHaveLength(1))
    control.voiceGate = gate().closed
    await answerByText('Перестанет быть оптимальным')
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(([url]) => url.endsWith('/t3/audio?part=reply')),
      ).toBe(true),
    )
    await act(() => vi.advanceTimersByTimeAsync(20_000))
    expect(screen.getByText('Верно про оптимальность.')).toBeVisible()

    // The next answer stops what is left of the old voice; the next reply
    // still waits for its own.
    await answerByText('Оценка расстояния')
    await waitFor(() =>
      expect(fetch.mock.calls.some(([url]) => url.includes('/t5/audio'))).toBe(
        true,
      ),
    )
    expect(screen.queryByText('эвристическая функция')).toBeNull()
    expect(screen.getByText('Geek думает…')).toBeVisible()
  } finally {
    vi.useRealTimers()
  }
})
