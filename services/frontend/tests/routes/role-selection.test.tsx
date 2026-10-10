import { fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { mockApiFetch } from '@/bootstrap/mock-api'
import { routeTree } from '@/routeTree.gen'
import { logOut } from '../support/account-menu'

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
  return router
}

it('opens student login with a banner and returns to role selection on logout', async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  const router = renderApp()
  expect(
    await screen.findByRole('button', { name: 'Войти как учитель' }),
  ).toBeVisible()
  expect(screen.queryByLabelText('Пароль')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Войти как ученик' }))
  expect(await screen.findByText('Вход как ученик')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Назад' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Войти как ученик' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Регистрация' }))
  expect(screen.getByText('Вход как ученик')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Вход' }))
  fireEvent.click(screen.getByRole('button', { name: 'Войти' }))
  expect(
    await screen.findByRole('button', { name: /Меню профиля/ }),
  ).toBeVisible()
  // Logout stays hidden until the profile menu is opened.
  expect(
    screen.queryByRole('button', { name: 'Выйти' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('menuitem', { name: 'Выйти' }),
  ).not.toBeInTheDocument()
  await logOut()
  expect(
    await screen.findByRole('button', { name: 'Войти как учитель' }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe('/welcome')
})

it.each(['real', 'mock', 'preview'])(
  'logs in as teacher without credentials in %s mode',
  async (mode) => {
    vi.stubEnv('VITE_API_MODE', mode)
    await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
    const fetch = vi.fn((url: string, options?: RequestInit) =>
      mockApiFetch(url, options),
    )
    vi.stubGlobal('fetch', fetch)
    const router = renderApp()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Войти как учитель' }),
    )
    expect(
      await screen.findByRole('heading', { name: 'Карта компетенций' }),
    ).toBeVisible()
    expect(screen.getByText('Учитель')).toBeVisible()
    expect(router.state.location.pathname).toBe('/admin/competency-map')
    expect(screen.queryByLabelText('Пароль')).not.toBeInTheDocument()
    if (mode === 'real') {
      const call = fetch.mock.calls.find(([url]) =>
        url.endsWith('/auth/teacher'),
      )!
      expect(call[1]?.method).toBe('POST')
      expect(call[1]?.body).toBeUndefined()
      expect(call[1]?.credentials).toBe('include')
    } else expect(fetch).not.toHaveBeenCalled()
    await logOut()
    expect(
      await screen.findByRole('button', { name: 'Войти как учитель' }),
    ).toBeVisible()
  },
)

it('keeps role selection available for retry when teacher login fails', async () => {
  vi.stubEnv('VITE_API_MODE', 'real')
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      url.endsWith('/auth/teacher')
        ? Response.json({ message: 'Сервер недоступен.' }, { status: 503 })
        : new Response(null, { status: 401 }),
    ),
  )
  renderApp()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Войти как учитель' }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Сервер недоступен.',
  )
  expect(screen.getByRole('button', { name: 'Войти как ученик' })).toBeEnabled()
  expect(
    screen.getByRole('button', { name: 'Войти как учитель' }),
  ).toBeEnabled()
})
