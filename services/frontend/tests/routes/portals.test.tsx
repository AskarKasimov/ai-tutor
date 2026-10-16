import { fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'
import { logOut } from '../support/account-menu'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  window.history.replaceState({}, '', '/')
})

function backend() {
  let user: {
    id: string
    email: string
    display_name: string
    role: string
  } | null = null
  const fetch = vi.fn(async (url: string, options?: RequestInit) => {
    if (
      (url.endsWith('/auth/teacher') || url.endsWith('/auth/login')) &&
      options?.method === 'POST'
    ) {
      const teacher = url.endsWith('/auth/teacher')
      user = {
        id: teacher ? 'teacher-1' : 'student-1',
        email: 'user@example.com',
        display_name: teacher ? 'Учитель' : 'Студент',
        role: teacher ? 'admin' : 'student',
      }
      return Response.json({ user })
    }
    if (url.endsWith('/auth/logout')) {
      user = null
      return new Response(null, { status: 204 })
    }
    if (url.endsWith('/auth/me'))
      return user ? Response.json(user) : new Response(null, { status: 401 })
    if (url.endsWith('/auth/refresh'))
      return new Response(null, { status: 401 })
    if (url.endsWith('/subjects')) return Response.json([])
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

// The browser address decides the portal, as it does for the API transport.
function renderApp(path: string) {
  window.history.replaceState({}, '', path)
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: [path] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  return router
}

it('opens the student site straight on sign-in and returns there on logout', async () => {
  backend()
  const router = renderApp('/')
  expect(await screen.findByLabelText('Пароль')).toBeVisible()
  expect(router.state.location.pathname).toBe('/login')
  expect(
    screen.getByText('Подтянем до уверенного уровня в кратчайшие сроки'),
  ).toBeVisible()
  // There is no role choice and no way into the teacher account.
  expect(screen.queryByText(/учител/i)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Регистрация' }))
  fireEvent.click(screen.getByRole('button', { name: 'Вход' }))
  expect(screen.getByLabelText('Email')).toBeRequired()
  expect(screen.getByLabelText('Пароль')).toBeRequired()
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'student@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'student-password' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Войти' }))
  expect(
    await screen.findByRole('button', { name: /Меню профиля/ }),
  ).toBeVisible()
  // Logout stays hidden until the profile menu is opened.
  expect(
    screen.queryByRole('menuitem', { name: 'Выйти' }),
  ).not.toBeInTheDocument()
  await logOut()
  expect(await screen.findByLabelText('Пароль')).toBeVisible()
  expect(router.state.location.pathname).toBe('/login')
})

it('opens the admin panel through the backend without a sign-in step', async () => {
  const fetch = backend()
  const router = renderApp('/admin')
  expect(
    await screen.findByRole('heading', { name: 'Подготовка курса' }),
  ).toBeVisible()
  expect(screen.getByText('Учитель')).toBeVisible()
  expect(router.state.location.pathname).toBe('/admin/competency-map')
  expect(screen.queryByLabelText('Пароль')).not.toBeInTheDocument()
  {
    const call = fetch.mock.calls.find(([url]) =>
      url.endsWith('/auth/teacher'),
    )!
    expect(call[1]?.method).toBe('POST')
    expect(call[1]?.body).toBeUndefined()
    expect(call[1]?.credentials).toBe('include')
    // The admin panel keeps its own session through the portal header.
    expect(new Headers(call[1]?.headers).get('X-AI-Tutor-Portal')).toBe('admin')
  }
  // The shared account has no logout.
  expect(
    screen.queryByRole('button', { name: /Меню профиля/ }),
  ).not.toBeInTheDocument()
})

it('offers a retry when the admin panel cannot sign in', async () => {
  const fetch = vi.fn(async (url: string) =>
    url.endsWith('/auth/teacher')
      ? Response.json({ message: 'Сервер недоступен.' }, { status: 503 })
      : new Response(null, { status: 401 }),
  )
  vi.stubGlobal('fetch', fetch)
  renderApp('/admin')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось открыть админ-панель.',
  )
  const attempts = () =>
    fetch.mock.calls.filter(([url]) => url.endsWith('/auth/teacher')).length
  expect(attempts()).toBe(1)
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(attempts()).toBe(2)
})
