import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { createQueryClient } from '../app/providers'
import { i18n } from '../i18n/i18n'
import { routeTree } from '../routeTree.gen'

function renderHome() {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id: 'existing', email: 'existing@example.com', display_name: 'Пользователь' }) }))
})

afterEach(async () => {
  vi.unstubAllGlobals()
  await i18n.changeLanguage('ru')
  i18n.removeResourceBundle('test', 'translation')
})

it('shows the single fixed question and microphone control', async () => {
  renderHome()
  expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent('Чем среднее арифметическое отличается от медианы?')
  expect(screen.getByRole('button', { name: 'Начать запись' })).toBeEnabled()
  expect(screen.queryByText('Следующее задание')).not.toBeInTheDocument()
})

it('previews an explicitly marked example and resets to the same question', async () => {
  renderHome()
  fireEvent.click(await screen.findByRole('button', { name: 'Посмотреть пример' }))
  expect(await screen.findByText('2 / 2', {}, { timeout: 2500 })).toBeVisible()
  expect(screen.getByText('Пример результата · не оценка вашей записи')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать ещё раз' }))
  expect(await screen.findByRole('button', { name: 'Начать запись' })).toBeEnabled()
  expect(screen.queryByText('2 / 2')).not.toBeInTheDocument()
})

it('keeps the question visible when microphone is unavailable', async () => {
  renderHome()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Микрофон недоступен')
  expect(screen.getByRole('heading', { level: 1 })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Попробовать снова' })).toBeEnabled()
})

it('renders the question through i18n', async () => {
  i18n.addResourceBundle('test', 'translation', { trainer: { question: 'A translated question' } })
  await i18n.changeLanguage('test')
  renderHome()
  expect(await screen.findByRole('heading', { level: 1, name: 'A translated question' })).toBeVisible()
})

function mockAuth(response: { ok: boolean; status?: number; body: unknown }) {
  const fetchMock = vi.fn().mockImplementation(async (_url, init) => {
    if (init?.method === 'POST') return { ok: response.ok, status: response.status, json: async () => response.body }
    return { ok: false, status: 401 }
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

it('logs in through the modal and updates the header', async () => {
  const fetchMock = mockAuth({ ok: true, body: { user: { id: '1', email: 'student@example.com', display_name: 'Студент' } } })
  renderHome()
  await screen.findByRole('dialog', { name: 'Вход в AI Tutor' })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled())
  expect(screen.getByRole('dialog', { name: 'Вход в AI Tutor' })).toBeVisible()
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'student@example.com' } })
  fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: 'my-password' } })
  fireEvent.submit(screen.getByRole('button', { name: 'Войти' }).closest('form')!)
  expect(await screen.findByText('Студент')).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  const [, init] = fetchMock.mock.calls.find(([url]) => url.endsWith('/auth/login'))!
  expect(init.credentials).toBe('include')
  expect(JSON.parse(init.body)).toEqual({ email: 'student@example.com', password: 'my-password' })
})

it('registers with backend password rules and an optional name', async () => {
  const fetchMock = mockAuth({ ok: true, body: { user: { id: '2', email: 'new@example.com', display_name: 'Новый' } } })
  renderHome()
  await screen.findByRole('dialog', { name: 'Вход в AI Tutor' })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: 'Регистрация' }))
  expect(screen.getByLabelText('Пароль')).toHaveAttribute('minlength', '15')
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } })
  fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: 'long-password-123' } })
  fireEvent.change(screen.getByLabelText('Имя (необязательно)'), { target: { value: 'Новый' } })
  fireEvent.submit(screen.getByRole('button', { name: 'Создать аккаунт' }).closest('form')!)
  expect(await screen.findByText('Новый')).toBeVisible()
  const [, init] = fetchMock.mock.calls.find(([url]) => url.endsWith('/auth/register'))!
  expect(JSON.parse(init.body)).toEqual({ email: 'new@example.com', password: 'long-password-123', display_name: 'Новый' })
})

it('keeps the modal open on failed login, Escape and overlay clicks', async () => {
  mockAuth({ ok: false, status: 401, body: { message: 'Неверный email или пароль.' } })
  renderHome()
  await screen.findByRole('dialog', { name: 'Вход в AI Tutor' })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled())
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'bad@example.com' } })
  fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: 'bad' } })
  fireEvent.submit(screen.getByRole('button', { name: 'Войти' }).closest('form')!)
  expect(await screen.findByRole('alert')).toHaveTextContent('Неверный email или пароль.')
  expect(screen.queryByRole('button', { name: 'Закрыть' })).not.toBeInTheDocument()
  const dialog = screen.getByRole('dialog')
  fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape', keyCode: 27 })
  fireEvent.mouseDown(dialog.parentElement!)
  fireEvent.mouseUp(dialog.parentElement!)
  fireEvent.click(dialog.parentElement!)
  expect(screen.getByRole('dialog')).toBeVisible()
})

it('opens the mandatory modal again after logout', async () => {
  const fetchMock = vi.fn().mockImplementation(async (_url, init) => init?.method === 'POST'
    ? { ok: true, status: 204 }
    : { ok: true, json: async () => ({ id: '1', email: 'student@example.com', display_name: 'Студент' }) })
  vi.stubGlobal('fetch', fetchMock)
  renderHome()
  expect(await screen.findByText('Студент')).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Выйти' }))
  expect(await screen.findByRole('dialog', { name: 'Вход в AI Tutor' })).toBeVisible()
})
