import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { StrictMode } from 'react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

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
      Response.json([{ id: 'subject-1', name: 'Предмет A', ready: true }]),
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
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  sessionStorage.clear()
  vi.stubGlobal('fetch', vi.fn(backendResponse))
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('returns from a completed learning plan to the course before restarting the diagnostic', async () => {
  const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
    /\/$/,
    '',
  )
  sessionStorage.setItem(
    'ai-tutor:learning-home',
    JSON.stringify({
      apiBase,
      userId: 'student-1',
      subjectId: 'subject-1',
      sessionId: '',
      diagnosticId: 'diagnostic-done',
      mode: 'training',
    }),
  )
  let stateReads = 0
  const requests = vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.endsWith('/learning-state')) {
      stateReads++
      return Response.json({
        subject_id: 'subject-1',
        subject_name: 'Предмет A',
        diagnostic_status: 'completed',
        diagnostic_completed: true,
        training_available: true,
        diagnostic_session_id: 'diagnostic-done',
      })
    }
    if (url.endsWith('/diagnostic-sessions/diagnostic-done/learning-plan'))
      return Response.json({
        id: 'finished-plan',
        diagnostic_session_id: 'diagnostic-done',
        subject_id: 'subject-1',
        subject_name: 'Предмет A',
        hours: 1,
        detail: 'brief',
        status: 'completed',
        current_index: null,
        progress_percent: 100,
        planned_minutes: 30,
        pending_answer: null,
        items: [
          {
            index: 0,
            competency_id: 'c1',
            competency_name: 'Модели',
            outcome_id: 'o1',
            outcome_name: 'Различать модели',
            theory_minutes: 15,
            practice_minutes: 15,
            in_plan: true,
            status: 'passed',
            attempts: 1,
          },
        ],
      })
    return backendResponse(url, init)
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  expect(await screen.findByText('План пройден!')).toBeVisible()

  const done = screen.getByRole('region', { name: 'План пройден!' })
  fireEvent.click(within(done).getByRole('button', { name: 'На главную' }))

  expect(
    await screen.findByRole('heading', { name: 'Курсы', level: 1 }),
  ).toBeVisible()
  const restart = await screen.findByRole('button', {
    name: 'Пройти проверку заново',
  })
  expect(sessionStorage.getItem('ai-tutor:learning-home')).toBeNull()
  await waitFor(() => expect(stateReads).toBeGreaterThan(1))
  expect(requests.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
    false,
  )

  fireEvent.click(restart)
  await waitFor(() =>
    expect(
      requests.mock.calls.some(
        ([url, init]) =>
          String(url).endsWith('/variants') && init?.method === 'POST',
      ),
    ).toBe(true),
  )
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
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const view = render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Предмет A',
      level: 2,
    }),
  ).toBeVisible()
  expect(
    screen.queryByRole('combobox', { name: 'Предмет' }),
  ).not.toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Курсы', level: 1 })).toBeVisible()
  expect(screen.getByRole('article', { name: 'Предмет A' })).toBeVisible()
  // Diagnostic and training are one process: a single action, no training tile.
  // Four steps say what the student does, starting with a voice check.
  const steps = screen.getByRole('list', { name: 'Как проходит обучение' })
  expect(steps).toHaveTextContent('Пройдите проверку')
  expect(steps).toHaveTextContent('Голосом, без клавиатуры')
  expect(steps).toHaveTextContent('Найдите пробелы')
  expect(steps).toHaveTextContent('Персонализированный под вас')
  expect(steps).toHaveTextContent('Подготовьтесь по плану')
  expect(
    screen.queryByRole('button', { name: /тренировк/i }),
  ).not.toBeInTheDocument()
  expect(
    await screen.findByRole('button', { name: 'Начать обучение' }),
  ).toBeEnabled()
  expect(requests.some((r) => r.includes('/learning-state'))).toBe(true)
  expect(requests.some((r) => r.endsWith('POST /api/v1/variants'))).toBe(false)
  const startButton = screen.getByRole('button', { name: 'Начать обучение' })
  expect(startButton).toHaveAccessibleDescription(
    'Проверка знаний, затем подготовка',
  )
  fireEvent.click(startButton)
  fireEvent.click(startButton)
  expect(
    await screen.findByRole('heading', {
      name: 'Вопрос о классификации',
    }),
  ).toBeVisible()
  await waitFor(() =>
    expect(requests.some((r) => r.endsWith('POST /api/v1/variants'))).toBe(
      true,
    ),
  )
  view.unmount()
  const refreshed = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={refreshed} />)
  expect(
    await screen.findByRole('heading', {
      name: 'Вопрос о классификации',
    }),
  ).toBeVisible()
  await waitFor(() =>
    expect(
      requests.some((r) => r.includes('GET /api/v1/diagnostic-sessions/')),
    ).toBe(true),
  )
  expect(
    requests.filter((r) => r.endsWith('POST /api/v1/variants')),
  ).toHaveLength(1)
  expect(api).toHaveBeenCalled()
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
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const first = render(<RouterProvider router={firstRouter} />)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать обучение' }),
  )
  await waitFor(() =>
    expect(requests.filter((r) => r.path.endsWith('/variants'))).toHaveLength(
      1,
    ),
  )

  first.unmount()
  const refreshedRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={refreshedRouter} />)
  expect(
    await screen.findByRole('heading', {
      name: 'Предмет A',
      level: 2,
    }),
  ).toBeVisible()
  await new Promise((resolve) => setTimeout(resolve, 150))
  expect(requests.filter((r) => r.path.endsWith('/variants'))).toHaveLength(1)

  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать обучение' }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Вопрос о классификации',
    }),
  ).toBeVisible()
  const variantRequests = requests.filter((r) => r.path.endsWith('/variants'))
  expect(variantRequests).toHaveLength(2)
  expect(variantRequests[1].key).toBe(variantRequests[0].key)
})
