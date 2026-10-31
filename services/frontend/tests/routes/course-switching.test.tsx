import { afterEach, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function installStorage() {
  const store = () => {
    const data = new Map<string, string>()
    return {
      getItem: (key: string) => data.get(key) ?? null,
      setItem: (key: string, value: string) => data.set(key, value),
      removeItem: (key: string) => data.delete(key),
      clear: () => data.clear(),
    }
  }
  vi.stubGlobal('localStorage', store())
  vi.stubGlobal('sessionStorage', store())
}

const courses = [
  {
    id: 'course-1',
    name: 'Курс один',
    ready: true,
    learning_ready: true,
    exam_ready: true,
  },
  {
    id: 'course-2',
    name: 'Курс два',
    ready: true,
    learning_ready: true,
    exam_ready: true,
  },
]

async function mount(
  path: '/learn' | '/exam',
  initialSearch = '?subject_id=course-1',
  setupStorage?: () => void,
) {
  installStorage()
  setupStorage?.()
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input) => {
      const url = String(input)
      calls.push(url)
      if (url.endsWith('/auth/me'))
        return Response.json({
          id: 'u1',
          email: 'u@example.com',
          display_name: 'Ученик',
          role: 'student',
        })
      if (url.endsWith('/subjects')) return Response.json(courses)
      if (url.includes('/learning-state'))
        return Response.json({
          subject_id: url.includes('course-2') ? 'course-2' : 'course-1',
          subject_name: 'Курс',
          diagnostic_status: 'not_started',
          diagnostic_completed: false,
          training_available: false,
        })
      if (url.includes('/diagnostic-sessions?')) return Response.json([])
      throw new Error(`Unexpected ${url}`)
    }),
  )
  const history = createMemoryHistory({
    initialEntries: [`${path}${initialSearch}`],
  })
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history,
    routeTree,
  })
  const renderResult = render(<RouterProvider router={router} />)
  return { calls, history, router, unmount: renderResult.unmount }
}

it.each(['/learn', '/exam'] as const)(
  '%s preserves the saved course ahead of a resumable session without a course panel',
  async (path) => {
    const key =
      path === '/learn' ? 'ai-tutor:learning-home' : 'ai-tutor:geek-view'
    const intent = JSON.stringify({
      apiBase: '/api/v1/learn',
      userId: 'u1',
      subjectId: 'course-1',
      sessionId: 'session-1',
      mode: 'diagnostic',
      screen: 'dialog',
    })
    const { calls } = await mount(path, '', () => {
      localStorage.setItem('ai-tutor:selected-subject:u1', 'course-2')
      sessionStorage.setItem(key, intent)
    })
    await waitFor(() =>
      expect(calls.some((url) => url.includes('course-2'))).toBe(true),
    )
    expect(sessionStorage.getItem(key)).toBe(intent)
    expect(
      screen.queryByRole('region', { name: 'Курс' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Выберите курс' }),
    ).not.toBeInTheDocument()
  },
)

it.each(['/learn', '/exam'] as const)(
  '%s loads the course specified in the URL',
  async (path) => {
    const { calls } = await mount(path, '?subject_id=course-2')
    await waitFor(() =>
      expect(calls.some((url) => url.includes('course-2'))).toBe(true),
    )
  },
)
