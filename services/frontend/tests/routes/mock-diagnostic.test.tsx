import { fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { mockApiFetch } from '@/bootstrap/mock-api'
import * as audio from '@/shared/lib'
import { routeTree } from '@/routeTree.gen'

vi.mock('@/shared/lib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/lib')>()),
  createVoiceWaveform: () => ({ setStream: () => {}, dispose: () => {} }),
}))

beforeEach(async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  sessionStorage.clear()
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  await mockApiFetch('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email: '', password: '' }),
  })
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    stop: vi
      .fn()
      .mockResolvedValue(new Blob(['audio'], { type: 'audio/webm' })),
    dispose: vi.fn(),
  })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('completes local diagnostic and displays its overall feedback', async () => {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать диагностику' }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Демонстрационный вопрос о классификации',
    }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Общий фидбэк' }),
  ).toBeVisible()
  expect(
    await screen.findByText('Вы уверенно различаете задачи классификации.'),
  ).toBeVisible()
  expect(
    screen.getByText('Диагностический балл').parentElement,
  ).toHaveTextContent('2 / 2')
  // The summary leads straight to training as the next stage.
  fireEvent.click(screen.getByRole('button', { name: 'Перейти к тренировке' }))
  expect(
    (await screen.findAllByRole('heading', { name: /Тренировка/ })).length,
  ).toBeGreaterThan(0)
  expect(
    screen.queryByRole('heading', { name: 'Общий фидбэк' }),
  ).not.toBeInTheDocument()
})
