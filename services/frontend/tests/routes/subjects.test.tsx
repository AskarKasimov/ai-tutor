import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import * as mockApi from '@/bootstrap/mock-api'
import { routeTree } from '@/routeTree.gen'

beforeEach(async () => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  vi.stubEnv('VITE_API_MODE', 'demo')
  sessionStorage.clear()
  await mockApi.mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  await mockApi.mockApiFetch('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email: '', password: '' }),
  })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('loads subjects without creating a variant, then starts only the chosen subject on explicit action', async () => {
  const requests: string[] = []
  const original = mockApi.mockApiFetch
  const api = vi
    .spyOn(mockApi, 'mockApiFetch')
    .mockImplementation(async (url, init) => {
      requests.push(
        `${init?.method ?? 'GET'} ${new URL(url, 'http://local').pathname}`,
      )
      return original(url, init)
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
      name: 'Демонстрационный предмет A',
      level: 2,
    }),
  ).toBeVisible()
  expect(
    screen.queryByRole('combobox', { name: 'Предмет' }),
  ).not.toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Курсы', level: 1 })).toBeVisible()
  expect(
    screen.getByRole('article', { name: 'Демонстрационный предмет A' }),
  ).toBeVisible()
  expect(
    await screen.findByRole('button', { name: 'Откроется после диагностики' }),
  ).toBeDisabled()
  // The hint stays only on the locked training tile, not on the diagnostic one.
  expect(screen.getAllByText('Сначала пройдите диагностику.')).toHaveLength(1)
  expect(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
  ).toBeEnabled()
  expect(requests.some((r) => r.includes('/learning-state'))).toBe(true)
  expect(requests.some((r) => r.endsWith('POST /api/v1/variants'))).toBe(false)
  const startButton = screen.getByRole('button', { name: 'Начать диагностику' })
  fireEvent.click(startButton)
  fireEvent.click(startButton)
  expect(
    await screen.findByRole('heading', {
      name: 'Демонстрационный вопрос о классификации',
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
      name: 'Демонстрационный вопрос о классификации',
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
  const original = mockApi.mockApiFetch
  vi.spyOn(mockApi, 'mockApiFetch').mockImplementation(
    async (url, init = {}) => {
      const path = new URL(url, 'http://local').pathname
      const method = init.method ?? 'GET'
      const key = new Headers(init.headers).get('Idempotency-Key') ?? undefined
      if (
        path.endsWith('/variants') &&
        method === 'POST' &&
        deferFirstVariant
      ) {
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
      return original(url, init)
    },
  )

  const firstRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const first = render(<RouterProvider router={firstRouter} />)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
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
      name: 'Демонстрационный предмет A',
      level: 2,
    }),
  ).toBeVisible()
  await new Promise((resolve) => setTimeout(resolve, 150))
  expect(requests.filter((r) => r.path.endsWith('/variants'))).toHaveLength(1)

  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Демонстрационный вопрос о классификации',
    }),
  ).toBeVisible()
  const variantRequests = requests.filter((r) => r.path.endsWith('/variants'))
  expect(variantRequests).toHaveLength(2)
  expect(variantRequests[1].key).toBe(variantRequests[0].key)
})

it('starts with a fresh variant after demo state resets between variant and session creation', async () => {
  const requests: Array<{ method: string; path: string }> = []
  const original = mockApi.mockApiFetch
  let interruptSessionStart = true
  vi.spyOn(mockApi, 'mockApiFetch').mockImplementation(
    async (url, init = {}) => {
      const path = new URL(url, 'http://local').pathname
      const method = init.method ?? 'GET'
      if (
        method === 'POST' &&
        path.endsWith('/diagnostic-sessions') &&
        interruptSessionStart
      ) {
        interruptSessionStart = false
        requests.push({ method, path })
        return Response.json({ code: 'TEMPORARY_FAILURE' }, { status: 500 })
      }
      if (
        method === 'POST' &&
        (path.endsWith('/auth/logout') || path.endsWith('/auth/login'))
      ) {
        const response = await original(url, init)
        requests.push({ method, path })
        return response
      }
      requests.push({ method, path })
      return original(url, init)
    },
  )

  const firstRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const first = render(<RouterProvider router={firstRouter} />)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
  )
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(
    requests.filter((r) => r.method === 'POST' && r.path.endsWith('/variants')),
  ).toHaveLength(1)
  expect(
    requests.filter(
      (r) => r.method === 'POST' && r.path.endsWith('/diagnostic-sessions'),
    ),
  ).toHaveLength(1)

  first.unmount()
  await original('/api/v1/auth/logout', { method: 'POST' })
  await original('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email: '', password: '' }),
  })
  const refreshedRouter = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={refreshedRouter} />)
  await new Promise((resolve) => setTimeout(resolve, 120))
  expect(
    requests.filter((r) => r.method === 'POST' && r.path.endsWith('/variants')),
  ).toHaveLength(1)
  expect(
    requests.filter(
      (r) => r.method === 'POST' && r.path.endsWith('/diagnostic-sessions'),
    ),
  ).toHaveLength(1)

  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Демонстрационный вопрос о классификации',
    }),
  ).toBeVisible()
  expect(
    requests.filter((r) => r.method === 'POST' && r.path.endsWith('/variants')),
  ).toHaveLength(2)
  expect(
    requests.filter(
      (r) => r.method === 'POST' && r.path.endsWith('/diagnostic-sessions'),
    ),
  ).toHaveLength(2)
})
