import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { createQueryClient } from '@/bootstrap/providers'
import * as assessment from '@/entities/assessment'
import * as voiceApi from '@/shared/api'
import { i18n } from '@/shared/i18n'
import * as audio from '@/shared/lib'
import { routeTree } from '@/routeTree.gen'

vi.mock('@/shared/lib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/lib')>()),
  createVoiceWaveform: () => ({ setStream: () => {}, dispose: () => {} }),
}))

function renderHome() {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

function renderDemoHome() {
  vi.stubEnv('VITE_API_MODE', 'mock')
  const queryClient = createQueryClient()
  queryClient.setQueryData(['auth', 'me'], {
    id: 'demo-test',
    email: 'student@example.com',
    display_name: 'Демо-студент',
    role: 'student',
  })
  const router = createRouter({
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        id: 'existing',
        email: 'existing@example.com',
        display_name: 'Пользователь',
      }),
    }),
  )
})

afterEach(async () => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('ru')
  i18n.removeResourceBundle('test', 'translation')
})

it('shows the model score and feedback for a recorded answer', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi
      .fn()
      .mockResolvedValue(new Blob(['audio'], { type: 'audio/webm' })),
  })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
  vi.spyOn(voiceApi, 'transcribeRecording').mockResolvedValue({
    id: 'tr-1',
    text: 'Классификация, потому что два класса.',
  })
  vi.spyOn(assessment, 'evaluateAnswer').mockResolvedValue({
    score: 2,
    feedback: [
      'Ответ верный.',
      'Вы назвали классификацию и объяснили два класса.',
      'Закрепите различие с регрессией.',
    ],
  })
  renderDemoHome()
  fireEvent.click(
    await screen.findByRole(
      'button',
      { name: 'Начать запись' },
      { timeout: 3000 },
    ),
  )
  fireEvent.click(
    await screen.findByRole(
      'button',
      { name: 'Завершить запись' },
      { timeout: 3000 },
    ),
  )
  expect(await screen.findByText('2 / 2')).toBeVisible()
  expect(
    screen.getByText('Классификация, потому что два класса.'),
  ).toBeVisible()
  const savedAnswer = screen.getByRole('region', { name: 'Ваш ответ' })
  expect(savedAnswer).toContainElement(
    screen.getByText('Классификация, потому что два класса.'),
  )
  expect(
    screen.getByRole('complementary', { name: 'Голосовой ответ на задание' }),
  ).not.toContainElement(savedAnswer)
  expect(
    screen.getByText('Вы назвали классификацию и объяснили два класса.'),
  ).toBeVisible()
})

it('shows the question, answer options and microphone control', async () => {
  renderDemoHome()
  expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
    'Банк по данным клиента прогнозирует: «вернёт кредит в срок» или «не вернёт».',
  )
  expect(screen.getByRole('heading', { name: 'Варианты ответа' })).toBeVisible()
  expect(screen.getByText('Классификация')).toBeVisible()
  expect(screen.getByText('Регрессия')).toBeVisible()
  expect(screen.getByText('Кластеризация')).toBeVisible()
  expect(screen.getByText('Ранжирование')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Повторить вопрос' })).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Начать запись' })).toBeEnabled()
  expect(screen.queryByText('Следующее задание')).not.toBeInTheDocument()
  expect(screen.getByRole('note', { name: 'dev-режим' })).toBeVisible()
})

it('shows the dev ribbon when the API mode is not real', async () => {
  vi.stubEnv('VITE_API_MODE', 'preview')
  renderHome()
  expect(await screen.findByText('dev-режим')).toBeVisible()
  expect(await screen.findByRole('button', { name: 'Войти' })).toBeEnabled()
  expect(
    screen.queryByRole('button', { name: 'Начать запись' }),
  ).not.toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'student@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'wrong-password' },
  })
  fireEvent.submit(
    screen.getByRole('button', { name: 'Войти' }).closest('form')!,
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Неверный email или пароль.',
  )
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: '' } })
  expect(screen.getByLabelText('Email')).toBeRequired()
  expect(screen.getByLabelText('Пароль')).toBeRequired()
  fireEvent.change(screen.getByLabelText('Пароль'), { target: { value: '' } })
  expect(screen.getByLabelText('Email')).not.toBeRequired()
  expect(screen.getByLabelText('Пароль')).not.toBeRequired()
  fireEvent.click(screen.getByRole('button', { name: 'Войти' }))
  expect(
    await screen.findByRole('button', { name: 'Начать запись' }),
  ).toBeEnabled()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('runs seven distinct assignments, reviews an earlier answer and restarts with an empty session', async () => {
  vi.spyOn(audio, 'startRecording').mockImplementation(async () => ({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['audio'])),
  }))
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
  vi.spyOn(voiceApi, 'transcribeRecording').mockResolvedValue({
    id: 'tr-1',
    text: 'Классификация, потому что два класса.',
  })
  const evaluate = vi.spyOn(assessment, 'evaluateAnswer').mockResolvedValue({
    score: 1,
    feedback: ['Частично.', 'Причина.', 'Совет.'],
  })
  renderDemoHome()
  await screen.findByRole('button', { name: 'Начать запись' })
  expect(screen.getByRole('button', { name: /Задание 2/ })).toBeDisabled()
  for (let index = 0; index < 7; index++) {
    fireEvent.click(screen.getByRole('button', { name: 'Начать запись' }))
    expect(screen.getByRole('button', { name: /Задание 1/ })).toBeDisabled()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Завершить запись' }),
    )
    const next = await screen.findByRole('button', {
      name: index === 6 ? 'Посмотреть итог' : 'Следующее задание',
    })
    fireEvent.click(next)
    if (index === 0) {
      expect(
        await screen.findByRole('heading', { level: 1 }),
      ).toHaveTextContent('цены квартиры')
      fireEvent.click(screen.getByRole('button', { name: /Задание 1/ }))
      expect(
        screen.getByText('Классификация, потому что два класса.'),
      ).toBeVisible()
      expect(screen.getByLabelText('Прослушать вашу запись')).toBeVisible()
      fireEvent.click(
        screen.getByRole('button', { name: 'Вернуться к текущему заданию' }),
      )
    }
  }
  expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
    'Сессия завершена',
  )
  expect(screen.getByText('7 / 14')).toBeVisible()
  expect(
    new Set(evaluate.mock.calls.map((call) => call[1].question)).size,
  ).toBe(7)
  expect(evaluate.mock.calls[1][1].options[0]).toBe('Регрессия')
  fireEvent.click(screen.getByRole('button', { name: 'Начать заново' }))
  expect(
    await screen.findByRole('button', { name: 'Начать запись' }),
  ).toBeEnabled()
  expect(screen.getByRole('button', { name: /Задание 2/ })).toBeDisabled()
  expect(screen.queryByText('7 / 14')).not.toBeInTheDocument()
})

it('does not advance after a grading error and retries the same transcript', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['audio'])),
  })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
  const transcribe = vi
    .spyOn(voiceApi, 'transcribeRecording')
    .mockResolvedValue({ id: 'tr-1', text: 'Мой ответ' })
  vi.spyOn(assessment, 'evaluateAnswer')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({
      score: 2,
      feedback: ['Верно.', 'Причина.', 'Совет.'],
    })
  renderDemoHome()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Повторить оценку' }),
  )
  expect(
    screen.queryByRole('button', { name: 'Следующее задание' }),
  ).not.toBeInTheDocument()
  expect(
    await screen.findByRole('button', { name: 'Следующее задание' }),
  ).toBeEnabled()
  expect(transcribe).toHaveBeenCalledTimes(1)
})

it('keeps the question visible when microphone is unavailable', async () => {
  renderDemoHome()
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Микрофон недоступен',
  )
  expect(screen.getByRole('heading', { level: 1 })).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Попробовать снова' }),
  ).toBeEnabled()
})

it('renders the question through i18n', async () => {
  i18n.addResourceBundle('test', 'translation', {
    trainer: { question: 'A translated question' },
  })
  await i18n.changeLanguage('test')
  renderDemoHome()
  expect(
    await screen.findByRole('heading', {
      level: 1,
      name: 'A translated question',
    }),
  ).toBeVisible()
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
  renderHome()
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

it('registers with backend password rules and an optional name', async () => {
  const fetchMock = mockAuth({
    ok: true,
    body: {
      user: { id: '2', email: 'new@example.com', display_name: 'Новый' },
    },
  })
  renderHome()
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Регистрация' }))
  expect(screen.getByLabelText('Пароль')).toHaveAttribute('minlength', '15')
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'new@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'long-password-123' },
  })
  fireEvent.change(screen.getByLabelText('Имя (необязательно)'), {
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
  renderHome()
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
  fireEvent.click(screen.getByRole('button', { name: 'Выйти' }))
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor' }),
  ).toBeVisible()
})
