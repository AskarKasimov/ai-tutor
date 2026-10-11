import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { mockApiFetch } from '@/bootstrap/mock-api'
import { routeTree } from '@/routeTree.gen'
import * as audio from '@/shared/lib'

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

it('opens training directly from the diagnostic result', async () => {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  render(<RouterProvider router={router} />)
  fireEvent.click(
    await screen.findByRole(
      'button',
      { name: 'Начать обучение' },
      { timeout: 5000 },
    ),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Завершить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Посмотреть итог' }),
  )
  const playedBeforeTraining = play.mock.calls.length
  fireEvent.click(
    await screen.findByRole('button', { name: 'Пройти тренировку' }),
  )
  expect(
    screen.queryByRole('button', { name: 'Начать тренировку' }),
  ).not.toBeInTheDocument()
  expect(
    await screen.findByRole('heading', {
      name: 'Различать классификацию и регрессию',
    }),
  ).toBeVisible()
  expect(screen.getByText(/Демонстрационный режим/)).toBeVisible()
  expect(screen.getByText(/Раунд 1/)).toBeVisible()
  // The exercise instruction is read aloud without a click.
  await waitFor(() =>
    expect(play.mock.calls.length).toBeGreaterThan(playedBeforeTraining),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: /Завершить запись/ }),
  )
  // The exercise answer is only confirmed: no score or feedback.
  expect(await screen.findByText('Ответ принят')).toBeVisible()
  expect(screen.queryByText(/Оценка/)).not.toBeInTheDocument()
  expect(
    screen.queryByText(
      'Ответ верный. Вы правильно определили тип задачи. Продолжайте применять этот критерий.',
    ),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText(
      'Как определить, что задача относится к классификации?',
      { selector: 'p' },
    ),
  ).not.toBeInTheDocument()
  expect(
    screen.getByText(
      'Это классификация, потому что результат относится к одному из двух классов.',
    ),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Следующее задание' }))
  expect(
    await screen.findByText(
      'Какой тип задачи предсказывает числовое значение?',
    ),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Пропустить вопрос' }))
  expect(await screen.findByText('Вопрос пропущен')).toBeVisible()
  expect(audio.startRecording).toHaveBeenCalledTimes(2)
  // Back home, the single course action resumes the process in training.
  fireEvent.click(screen.getByRole('button', { name: 'На главную' }))
  expect(
    await screen.findByText('Проверка пройдена — продолжайте тренировку.'),
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Пройти проверку заново' }),
  ).toHaveAccessibleDescription('С первого вопроса')
  expect(
    screen.getByRole('button', { name: 'Продолжить обучение' }),
  ).toHaveAccessibleDescription('С того же места')
})
