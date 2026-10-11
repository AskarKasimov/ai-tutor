import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'

import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'
import { logOut } from '../support/account-menu'

vi.mock('@/shared/lib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/lib')>()),
  createVoiceWaveform: () => ({ setStream: () => {}, dispose: () => {} }),
}))

function renderHome(path = '/') {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: [path] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function mockAuth(response: { ok: boolean; status?: number; body: unknown }) {
  const fetchMock = vi.fn().mockImplementation(async (_url, init) => {
    if (init?.method === 'POST')
      return {
        ok: response.ok,
        status: response.status,
        json: async () => response.body,
      }
    return { ok: false, status: 401 }
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

it('logs in through the separate screen and updates the header', async () => {
  const fetchMock = mockAuth({
    ok: true,
    body: {
      user: { id: '1', email: 'student@example.com', display_name: 'Студент' },
    },
  })
  renderHome('/login')
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled(),
  )
  expect(screen.getByRole('heading', { name: 'Вход в AI Tutor' })).toBeVisible()
  expect(screen.getByLabelText('Email')).toBeRequired()
  expect(screen.getByLabelText('Пароль')).toBeRequired()
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'student@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'my-password' },
  })
  fireEvent.submit(
    screen.getByRole('button', { name: 'Войти' }).closest('form')!,
  )
  expect(await screen.findByText('Студент')).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  const [, init] = fetchMock.mock.calls.find(([url]) =>
    url.endsWith('/auth/login'),
  )!
  expect(init.credentials).toBe('include')
  expect(JSON.parse(init.body)).toEqual({
    email: 'student@example.com',
    password: 'my-password',
  })
})

it('registers with backend password rules and a required name', async () => {
  const fetchMock = mockAuth({
    ok: true,
    body: {
      user: { id: '2', email: 'new@example.com', display_name: 'Новый' },
    },
  })
  renderHome('/login')
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Регистрация' }))
  expect(screen.getByLabelText('Пароль')).toHaveAttribute('minlength', '15')
  expect(screen.getByLabelText('Имя')).toBeRequired()
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'new@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'long-password-123' },
  })
  fireEvent.change(screen.getByLabelText('Имя'), {
    target: { value: 'Новый' },
  })
  fireEvent.submit(
    screen.getByRole('button', { name: 'Создать аккаунт' }).closest('form')!,
  )
  expect(await screen.findByText('Новый')).toBeVisible()
  const [, init] = fetchMock.mock.calls.find(([url]) =>
    url.endsWith('/auth/register'),
  )!
  expect(JSON.parse(init.body)).toEqual({
    email: 'new@example.com',
    password: 'long-password-123',
    display_name: 'Новый',
  })
})

it('keeps the login screen visible after failed login, Escape and background clicks', async () => {
  mockAuth({
    ok: false,
    status: 401,
    body: { message: 'Неверный email или пароль.' },
  })
  renderHome('/login')
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled(),
  )
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'bad@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'bad' },
  })
  fireEvent.submit(
    screen.getByRole('button', { name: 'Войти' }).closest('form')!,
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Неверный email или пароль.',
  )
  expect(
    screen.queryByRole('button', { name: 'Закрыть' }),
  ).not.toBeInTheDocument()
  const dialog = screen.getByRole('region', { name: 'Вход в AI Tutor' })
  fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape', keyCode: 27 })
  fireEvent.mouseDown(dialog.parentElement!)
  fireEvent.mouseUp(dialog.parentElement!)
  fireEvent.click(dialog.parentElement!)
  expect(screen.getByRole('heading', { name: 'Вход в AI Tutor' })).toBeVisible()
})

it('returns to the separate login screen after logout', async () => {
  const fetchMock = vi.fn().mockImplementation(async (_url, init) =>
    init?.method === 'POST'
      ? { ok: true, status: 204 }
      : {
          ok: true,
          json: async () => ({
            id: '1',
            email: 'student@example.com',
            display_name: 'Студент',
          }),
        },
  )
  vi.stubGlobal('fetch', fetchMock)
  renderHome()
  expect(await screen.findByText('Студент')).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await logOut()
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor' }),
  ).toBeVisible()
})
