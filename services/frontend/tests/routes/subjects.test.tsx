import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
  const view = render(<RouterProvider router={router} />)
  expect(await screen.findByRole('combobox', { name: 'Предмет' })).toBeVisible()
  expect(requests.some((r) => r.includes('/learning-state'))).toBe(false)
  fireEvent.click(screen.getByRole('combobox', { name: 'Предмет' }))
  fireEvent.click(await screen.findByRole('option', { name: 'Введение в ML' }))
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
