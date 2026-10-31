import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'
import { competencyMapQueryKeys } from '@/features/import-learning-competency-map'

function backendResponse(url: string, init?: RequestInit): Promise<Response> {
  const current = {
    variant_task_id: 'v-main',
    source_task_id: 'bank-main',
    role: 'main',
    competency_id: 'c1',
    competency_name: 'Модели',
    constituent_id: 's1',
    constituent_name: 'Выбор модели',
    outcome_id: 'o1',
    outcome_name: 'Различать модели',
    question: 'Вопрос о классификации',
    options: ['Классификация', 'Регрессия'],
    voice_instruction: 'Назовите модель и объясните решение.',
  }
  const progress = {
    session_id: 'session-1',
    status: 'active',
    completed_tasks: 0,
    skipped_tasks: 0,
    total_tasks: 1,
    current,
  }
  if (url.endsWith('/auth/me'))
    return Promise.resolve(
      Response.json({
        id: 'student-1',
        email: 'student@example.com',
        display_name: 'Студент',
        role: 'student',
      }),
    )
  if (url.endsWith('/subjects'))
    return Promise.resolve(
      Response.json([
        {
          id: 'subject-1',
          name: 'Предмет A',
          ready: true,
          learning_ready: true,
          exam_ready: true,
        },
      ]),
    )
  if (url.endsWith('/diagnostic-outline'))
    return Promise.resolve(
      Response.json({
        competencies: [
          {
            id: 'c1',
            name: 'Модели машинного обучения',
            constituents: [
              {
                id: 't1',
                name: 'Выбор модели',
                outcomes: [
                  {
                    id: 'o1',
                    name: 'Различать классификацию и регрессию',
                    role: 'main',
                  },
                ],
              },
            ],
          },
        ],
      }),
    )
  if (url.endsWith('/learning-state'))
    return Promise.resolve(
      Response.json({
        subject_id: 'subject-1',
        subject_name: 'Предмет A',
        diagnostic_status: 'not_started',
        diagnostic_completed: false,
        training_available: false,
      }),
    )
  if (url.endsWith('/variants') && init?.method === 'POST')
    return Promise.resolve(Response.json({ id: 'variant-1' }, { status: 201 }))
  if (
    url.endsWith('/diagnostic-sessions') ||
    url.endsWith('/diagnostic-sessions/session-1')
  )
    return Promise.resolve(Response.json(progress))
  if (url.includes('/current/audio?'))
    return Promise.resolve(
      Response.json({
        variant_task_id: 'v-main',
        status: 'missing',
      }),
    )
  throw new Error(`Unexpected request: ${url}`)
}

beforeEach(() => {
  const localStore = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => localStore.get(key) ?? null,
    setItem: (key: string, value: string) => localStore.set(key, value),
    removeItem: (key: string) => localStore.delete(key),
    clear: () => localStore.clear(),
  })
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  sessionStorage.clear()
  localStorage.clear()
  vi.stubGlobal('fetch', vi.fn(backendResponse))
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('loads subjects without creating a variant, then starts only the chosen subject on explicit action', async () => {
  const requests: string[] = []
  const original = backendResponse
  const api = vi.mocked(fetch).mockImplementation(async (url, init) => {
    requests.push(
      `${init?.method ?? 'GET'} ${new URL(String(url), 'http://local').pathname}`,
    )
    return original(String(url), init)
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  const view = render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  )
  expect(
    await screen.findByRole('heading', { name: 'Обучение', level: 1 }),
  ).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Курс' })).not.toBeInTheDocument()
  expect(
    screen.queryByRole('navigation', { name: 'Направления' }),
  ).not.toBeInTheDocument()
  // Diagnostic and training are one process: a single action, no training tile.
  // Four steps say what the student does, starting with a voice check.
  const steps = await screen.findByRole('list', {
    name: 'Как проходит обучение',
  })
  expect(steps).toHaveTextContent('Пройдите проверку')
  expect(steps).toHaveTextContent('Голосом, без клавиатуры')
  expect(steps).toHaveTextContent('Найдите пробелы')
  expect(steps).toHaveTextContent('Персонализированный под вас')
  expect(steps).toHaveTextContent('Подготовьтесь по плану')
  expect(steps).toHaveTextContent('Подтяните слабые места')
  expect(steps).not.toHaveTextContent('Только нужное к экзамену')
  expect(
    screen.queryByRole('button', { name: /тренировк/i }),
  ).not.toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Начать обучение' }),
    ).toBeEnabled(),
  )
  expect(requests.some((r) => r.includes('/learning-state'))).toBe(true)
  expect(requests.some((r) => r.endsWith('POST /api/v1/learn/variants'))).toBe(
    false,
  )
  expect(
    await screen.findByRole('heading', { name: 'Ваш путь обучения' }),
  ).toBeVisible()
  expect(
    await screen.findByLabelText('Модели машинного обучения'),
  ).toBeInTheDocument()
  fireEvent.click(await screen.findByLabelText('Выбор модели'))
  expect(screen.getByText('Различать классификацию и регрессию')).toBeVisible()
  expect(screen.queryByText('Скрытый результат')).not.toBeInTheDocument()
  expect(requests).toContain(
    'GET /api/v1/learn/subjects/subject-1/diagnostic-outline',
  )
  const startButton = screen.getByRole('button', { name: 'Начать обучение' })
  expect(startButton).toHaveAccessibleDescription(
    'Проверка знаний, затем подготовка',
  )
  await waitFor(() => expect(startButton).toBeEnabled())
  fireEvent.click(startButton)
  await waitFor(() => expect(startButton).toBeEnabled())
  fireEvent.click(startButton)
  expect(
    await screen.findByRole('heading', {
      name: 'Вопрос о классификации',
    }),
  ).toBeVisible()
  await waitFor(() =>
    expect(
      requests.some((r) => r.endsWith('POST /api/v1/learn/variants')),
    ).toBe(true),
  )
  const sessionReadsBeforeRefresh = requests.filter((r) =>
    r.includes('GET /api/v1/learn/diagnostic-sessions/'),
  ).length
  view.unmount()
  const refreshed = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={refreshed} />)
  expect(
    await screen.findByRole('heading', { name: 'Обучение', level: 1 }),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Вопрос о классификации' }),
  ).not.toBeInTheDocument()
  expect(
    await screen.findByRole('button', { name: 'Начать обучение' }),
  ).toBeVisible()
  expect(
    requests.filter((r) =>
      r.includes('GET /api/v1/learn/diagnostic-sessions/'),
    ),
  ).toHaveLength(sessionReadsBeforeRefresh)
  expect(
    requests.filter((r) => r.endsWith('POST /api/v1/learn/variants')),
  ).toHaveLength(1)
  expect(api).toHaveBeenCalled()
})

it('starts the diagnostic when saving the page intent to sessionStorage fails', async () => {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  const startButton = await screen.findByRole('button', {
    name: 'Начать обучение',
  })
  vi.spyOn(sessionStorage, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage disabled', 'SecurityError')
  })

  await waitFor(() => expect(startButton).toBeEnabled())
  fireEvent.click(startButton)

  expect(
    await screen.findByRole('heading', { name: 'Вопрос о классификации' }),
  ).toBeVisible()
})

it('shows a retryable learning-state error while restoring a historical subject', async () => {
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.endsWith('/subjects')) return Response.json([])
    if (url.includes('/learning-state'))
      return new Response(null, { status: 503 })
    return backendResponse(url, init)
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({
      initialEntries: ['/learn?subject_id=historic-subject'],
    }),
    routeTree,
  })
  render(<RouterProvider router={router} />)

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось загрузить состояние обучения.',
  )
  expect(
    screen.getByRole('button', { name: 'Попробовать снова' }),
  ).toBeEnabled()
})

it('shows loading and a retryable error for the previously selected historical subject', async () => {
  localStorage.setItem(
    'ai-tutor:selected-subject:student-1',
    'historic-subject',
  )
  let finishFirst!: (response: Response) => void
  let stateRequests = 0
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.endsWith('/subjects')) return Response.json([])
    if (url.includes('/learning-state')) {
      stateRequests += 1
      if (stateRequests === 1)
        return new Promise<Response>((resolve) => {
          finishFirst = resolve
        })
      if (stateRequests === 2) return new Response(null, { status: 503 })
      return Response.json({
        subject_id: 'historic-subject',
        subject_name: 'Исторический курс',
        diagnostic_status: 'not_started',
        diagnostic_completed: false,
        training_available: false,
      })
    }
    return backendResponse(url, init)
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)

  await waitFor(() => expect(finishFirst).toBeDefined())
  expect(await screen.findByText('Загружаем состояние обучения…')).toBeVisible()
  finishFirst(new Response(null, { status: 503 }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось загрузить состояние обучения.',
  )
  expect(
    screen.getByRole('button', { name: 'Попробовать снова' }),
  ).toBeEnabled()
})

it('returns to subject selection after refresh during a pending start and reuses its key only after a new click', async () => {
  const requests: Array<{ method: string; path: string; key?: string }> = []
  let deferFirstVariant = true
  const original = backendResponse
  vi.mocked(fetch).mockImplementation(async (url, init = {}) => {
    const path = new URL(String(url), 'http://local').pathname
    const method = init.method ?? 'GET'
    const key = new Headers(init.headers).get('Idempotency-Key') ?? undefined
    if (path.endsWith('/variants') && method === 'POST' && deferFirstVariant) {
      deferFirstVariant = false
      requests.push({ method, path, key })
      return new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener(
          'abort',
          () => reject(new DOMException('Aborted', 'AbortError')),
          { once: true },
        )
      })
    }
    requests.push({ method, path, key })
    return original(String(url), init)
  })

  const firstRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  const first = render(<RouterProvider router={firstRouter} />)
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Начать обучение' }),
    ).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Начать обучение' }))
  await waitFor(() =>
    expect(requests.filter((r) => r.path.endsWith('/variants'))).toHaveLength(
      1,
    ),
  )

  first.unmount()
  const refreshedRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={refreshedRouter} />)
  expect(
    await screen.findByRole('heading', { name: 'Обучение', level: 1 }),
  ).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Курс' })).not.toBeInTheDocument()
  await new Promise((resolve) => setTimeout(resolve, 150))
  expect(requests.filter((r) => r.path.endsWith('/variants'))).toHaveLength(1)

  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Начать обучение' }),
    ).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Начать обучение' }))
  expect(
    await screen.findByRole('heading', {
      name: 'Вопрос о классификации',
    }),
  ).toBeVisible()
  const variantRequests = requests.filter((r) => r.path.endsWith('/variants'))
  expect(variantRequests).toHaveLength(2)
  expect(variantRequests[1].key).toBe(variantRequests[0].key)
})

it('previews the current map before restarting a diagnostic from training', async () => {
  const requests: string[] = []
  vi.mocked(fetch).mockImplementation(async (url, init) => {
    const path = String(url)
    requests.push(`${init?.method ?? 'GET'} ${path}`)
    if (path.endsWith('/learning-state'))
      return Response.json({
        subject_id: 'subject-1',
        subject_name: 'Предмет A',
        diagnostic_status: 'completed',
        diagnostic_completed: true,
        training_available: true,
        diagnostic_session_id: 'previous-diagnostic',
      })
    return backendResponse(path, init)
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  expect(
    await screen.findByRole('button', { name: 'Продолжить обучение' }),
  ).toBeEnabled()
  expect(
    await screen.findByRole('heading', { name: 'Ваш путь обучения' }),
  ).toBeVisible()
  expect(await screen.findByLabelText('Выбор модели')).toBeInTheDocument()
  expect(
    requests.some((r) => r.startsWith('POST ') && r.endsWith('/variants')),
  ).toBe(false)
  fireEvent.click(
    screen.getByRole('button', { name: 'Пройти проверку заново' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Вопрос о классификации' }),
  ).toBeVisible()
  expect(
    requests.filter((r) => r.startsWith('POST ') && r.endsWith('/variants')),
  ).toHaveLength(1)
})

it.each(['start', 'training', 'completed'] as const)(
  'hides a cached outline and gates a new diagnostic during refresh and retry at %s',
  async (stage) => {
    const startName =
      stage === 'start' ? 'Начать обучение' : 'Пройти проверку заново'
    const response = (url: string, init?: RequestInit) => {
      if (url.endsWith('/learning-state'))
        return Promise.resolve(
          Response.json({
            subject_id: 'subject-1',
            subject_name: 'Предмет A',
            diagnostic_status: stage === 'start' ? 'not_started' : 'completed',
            diagnostic_completed: stage !== 'start',
            training_available: stage === 'training',
            ...(stage === 'training'
              ? { diagnostic_session_id: 'previous-diagnostic' }
              : {}),
          }),
        )
      return backendResponse(url, init)
    }
    vi.mocked(fetch).mockImplementation((url, init) =>
      response(String(url), init),
    )
    const client = createQueryClient()
    const router = createRouter({
      context: { queryClient: client },
      history: createMemoryHistory({ initialEntries: ['/learn'] }),
      routeTree,
    })
    render(<RouterProvider router={router} />)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: startName })).toBeEnabled(),
    )
    expect(await screen.findByLabelText('Выбор модели')).toBeInTheDocument()

    let finish!: (response: Response) => void
    vi.mocked(fetch).mockImplementation(async (url, init) => {
      if (String(url).endsWith('/diagnostic-outline'))
        return new Promise<Response>((resolve) => {
          finish = resolve
        })
      return response(String(url), init)
    })
    const refresh = client.invalidateQueries({
      queryKey: competencyMapQueryKeys.outline('student-1', 'subject-1'),
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: startName })).toBeDisabled(),
    )
    expect(screen.queryByLabelText('Выбор модели')).not.toBeInTheDocument()
    if (stage === 'training')
      expect(
        screen.getByRole('button', { name: 'Продолжить обучение' }),
      ).toBeEnabled()
    expect(screen.getByRole('status')).toHaveTextContent('Загружаем план')
    fireEvent.click(screen.getByRole('button', { name: startName }))
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(
          ([url, init]) =>
            String(url).endsWith('/variants') && init?.method === 'POST',
        ),
    ).toBe(false)

    finish(new Response(null, { status: 503 }))
    await refresh
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Повторите загрузку',
    )
    expect(screen.getByRole('button', { name: startName })).toBeDisabled()
    expect(screen.queryByLabelText('Выбор модели')).not.toBeInTheDocument()
    if (stage === 'training')
      expect(
        screen.getByRole('button', { name: 'Продолжить обучение' }),
      ).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Загружаем план'),
    )
    finish(
      Response.json({
        competencies: [
          {
            id: 'new',
            name: 'Новая компетенция',
            constituents: [
              {
                id: 'new-topic',
                name: 'Актуальная тема',
                outcomes: [
                  { id: 'new-outcome', name: 'Новый результат', role: 'main' },
                ],
              },
            ],
          },
        ],
      }),
    )
    expect(await screen.findByLabelText('Актуальная тема')).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: startName })).toBeEnabled(),
    )
    expect(screen.queryByLabelText('Выбор модели')).not.toBeInTheDocument()
    if (stage === 'training')
      expect(
        screen.getByRole('button', { name: 'Продолжить обучение' }),
      ).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: startName }))
    expect(
      await screen.findByRole('heading', { name: 'Вопрос о классификации' }),
    ).toBeVisible()
  },
)
