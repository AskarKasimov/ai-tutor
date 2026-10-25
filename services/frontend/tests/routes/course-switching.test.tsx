import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
) {
  installStorage()
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
  const select = await screen.findByRole('combobox', { name: 'Выберите курс' })
  return { calls, history, router, select, unmount: renderResult.unmount }
}

it.each(['/learn', '/exam'] as const)(
  '%s honors direct course URL, explicit selection, reload URL, Back and Forward',
  async (path) => {
    const { calls, history, router, select, unmount } = await mount(path)
    expect(select).toHaveTextContent('Курс один')
    fireEvent.click(select)
    fireEvent.click(await screen.findByRole('option', { name: 'Курс два' }))
    await waitFor(() =>
      expect(router.state.location.search.subject_id).toBe('course-2'),
    )
    await waitFor(() => expect(select).toHaveTextContent('Курс два'))
    expect(calls.some((url) => url.includes('course-2'))).toBe(true)

    history.back()
    await waitFor(() =>
      expect(router.state.location.search.subject_id).toBe('course-1'),
    )
    await waitFor(() => expect(select).toHaveTextContent('Курс один'))
    history.forward()
    await waitFor(() =>
      expect(router.state.location.search.subject_id).toBe('course-2'),
    )
    await waitFor(() => expect(select).toHaveTextContent('Курс два'))
    unmount()
    const reloaded = await mount(path, '?subject_id=course-2')
    expect(reloaded.select).toHaveTextContent('Курс два')
    expect(reloaded.calls.some((url) => url.includes('course-2'))).toBe(true)
    reloaded.unmount()
  },
)

it.each(['/learn', '/exam'] as const)(
  '%s selection works without an initial subject_id',
  async (path) => {
    const { router, select } = await mount(path, '')
    expect(select).toHaveTextContent('Курс один')
    fireEvent.click(select)
    fireEvent.click(await screen.findByRole('option', { name: 'Курс два' }))
    await waitFor(() =>
      expect(router.state.location.search.subject_id).toBe('course-2'),
    )
    await waitFor(() => expect(select).toHaveTextContent('Курс два'))
  },
)

it.each(['/learn', '/exam'] as const)(
  '%s does not fall back when a direct subject_id is unknown',
  async (path) => {
    const { select, unmount } = await mount(path, '?subject_id=missing-course')
    expect(select).not.toHaveTextContent('Курс один')
    expect(select).not.toHaveTextContent('Курс два')
    unmount()
  },
)
