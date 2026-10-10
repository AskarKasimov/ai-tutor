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

it('loads a focused preview before starting and explicitly starts the frozen plan', async () => {
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
      { name: 'Начать диагностику' },
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
  fireEvent.click(await screen.findByRole('button', { name: 'На главную' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Открыть тренировку' }),
  )

  expect(
    await screen.findByRole('heading', { name: 'Неосвоенные темы' }),
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Частичные пробелы в знаниях' }),
  ).toBeVisible()
  expect(screen.getByText('Диагностический балл: 2 / 2')).toBeVisible()
  const playedBeforeTraining = play.mock.calls.length
  fireEvent.click(screen.getByRole('button', { name: 'Начать тренировку' }))
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
  expect(await screen.findByText('Оценка: 2 / 2')).toBeVisible()
  const feedbackParagraph = screen.getByText(
    'Ответ верный. Вы правильно определили тип задачи. Продолжайте применять этот критерий.',
  )
  expect(feedbackParagraph).toBeVisible()
  expect(feedbackParagraph.tagName).toBe('P')
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
  fireEvent.click(screen.getByRole('button', { name: 'Следующий раунд' }))
  expect(
    await screen.findByText(
      'Какой тип задачи предсказывает числовое значение?',
    ),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Пропустить вопрос' }))
  expect(
    await screen.findByText(
      'Вопрос пропущен. Ответ оценён в 0 баллов. Продолжите со следующим вопросом.',
    ),
  ).toBeVisible()
  expect(audio.startRecording).toHaveBeenCalledTimes(2)
})
