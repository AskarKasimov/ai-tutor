import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

const chunk = vi.hoisted(() => ({
  release: undefined as (() => void) | undefined,
}))
vi.mock(
  '@/pages/learning-trainer/ui/diagnostic-roadmap-canvas',
  async (importOriginal) => {
    await new Promise<void>((resolve) => {
      chunk.release = resolve
    })
    return importOriginal()
  },
)

afterEach(() => {
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

it('keeps new diagnostics disabled until the lazy graph has actually mounted', async () => {
  const values = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  })
  const task = {
    variant_task_id: 'v',
    source_task_id: 'bank',
    role: 'main',
    competency_id: 'c',
    competency_name: 'Модели',
    constituent_id: 'topic',
    constituent_name: 'Нейронные сети',
    outcome_id: 'outcome',
    outcome_name: 'Понимать нейросети',
    question: 'Как устроена нейронная сеть?',
    options: [],
    voice_instruction: 'Ответьте устно.',
  }
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/auth/me'))
        return Response.json({
          id: 'student',
          email: 'student@example.com',
          display_name: null,
          role: 'student',
        })
      if (url.endsWith('/subjects'))
        return Response.json([
          {
            id: 'course',
            name: 'Курс',
            ready: true,
            learning_ready: true,
            exam_ready: true,
          },
        ])
      if (url.endsWith('/learning-state'))
        return Response.json({
          subject_id: 'course',
          subject_name: 'Курс',
          diagnostic_status: 'not_started',
          diagnostic_completed: false,
          training_available: false,
        })
      if (url.endsWith('/diagnostic-outline'))
        return Response.json({
          competencies: [
            {
              id: 'c',
              name: 'Модели',
              constituents: [
                {
                  id: 'topic',
                  name: 'Нейронные сети',
                  outcomes: [
                    { id: 'outcome', name: 'Понимать нейросети', role: 'main' },
                  ],
                },
              ],
            },
          ],
        })
      if (url.endsWith('/variants') && init?.method === 'POST')
        return Response.json({ id: 'variant' }, { status: 201 })
      if (url.endsWith('/diagnostic-sessions'))
        return Response.json(
          {
            session_id: 'session',
            status: 'active',
            completed_tasks: 0,
            skipped_tasks: 0,
            total_tasks: 1,
            current: task,
          },
          { status: 201 },
        )
      if (url.includes('/current/audio?'))
        return Response.json({ variant_task_id: 'v', status: 'missing' })
      throw new Error(`Unexpected request: ${url}`)
    },
  )
  vi.stubGlobal('fetch', fetchMock)
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/learn'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)

  // The chunk is requested only after a successful, nonempty outline response.
  await waitFor(() => expect(chunk.release).toBeTypeOf('function'))
  const start = screen.getByRole('button', { name: 'Начать обучение' })
  expect(start).toBeDisabled()
  expect(screen.getByRole('status')).toHaveTextContent('Загружаем план')
  expect(screen.queryByLabelText('Нейронные сети')).not.toBeInTheDocument()
  fireEvent.click(start)
  expect(
    fetchMock.mock.calls.some(
      ([url, init]) =>
        String(url).endsWith('/variants') && init?.method === 'POST',
    ),
  ).toBe(false)

  chunk.release!()
  expect(await screen.findByLabelText('Нейронные сети')).toBeInTheDocument()
  await waitFor(() => expect(start).toBeEnabled())
  fireEvent.click(start)
  expect(
    await screen.findByRole('heading', {
      name: 'Как устроена нейронная сеть?',
    }),
  ).toBeVisible()
  expect(
    fetchMock.mock.calls.filter(
      ([url, init]) =>
        String(url).endsWith('/variants') && init?.method === 'POST',
    ),
  ).toHaveLength(1)
})
