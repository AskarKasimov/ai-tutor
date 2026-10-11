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
  expect(
    await screen.findByRole('heading', { name: 'Репетитор' }),
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Записать голосовую реплику' }),
  ).toBeVisible()
  expect(screen.getByRole('img', { name: 'Визуализация записи' })).toBeVisible()
  const workspace = screen.getByRole('main')
  expect(workspace).toHaveAttribute('data-variant', 'training')
  expect(screen.getByRole('heading', { name: 'Цели тренировки' })).toBeVisible()
  const input = screen.getByRole('textbox', { name: 'Сообщение репетитору' })
  fireEvent.change(input, { target: { value: 'Что означает классификация?' } })
  fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))
  expect(
    await screen.findByText(
      'Вы назвали важную часть рассуждения. Какое свойство результата подтверждает ваш выбор?',
    ),
  ).toBeVisible()
  expect(screen.getByText(/Раунд 1/)).toBeVisible()
  expect(screen.queryByText(/Оценка:/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Нужна подсказка' }))
  expect(
    await screen.findByText(
      'Подскажи следующий шаг, не раскрывая готовый ответ.',
    ),
  ).toBeVisible()
  fireEvent.change(input, {
    target: { value: 'Классификация выбирает один из классов.' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Проверить ответ' }))
  expect(await screen.findByText('Ответ проверен')).toBeVisible()
  expect(screen.queryByText(/Результат проверки:/)).not.toBeInTheDocument()
  expect(screen.queryByText(/2 \/ 2/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Следующий раунд' }))
  expect(
    await screen.findByText(
      'Какой тип задачи предсказывает числовое значение?',
    ),
  ).toBeVisible()
  fireEvent.click(await screen.findByRole('button', { name: 'Открыть диалог' }))
  expect(
    await screen.findAllByText('Классификация выбирает один из классов.'),
  ).toHaveLength(2)
  expect(
    screen.getAllByRole('textbox', { name: 'Сообщение репетитору' }),
  ).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Пропустить упражнение' }))
  expect(
    await screen.findByText(
      'Как определить, что задача относится к классификации?',
    ),
  ).toBeVisible()
  expect(audio.startRecording).toHaveBeenCalledTimes(1)
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
