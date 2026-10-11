import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { mockApiFetch } from '@/bootstrap/mock-api'
import { routeTree } from '@/routeTree.gen'
import * as audio from '@/shared/lib'
import { mockChatVoice } from '../support/mock-chat-voice'

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
  mockChatVoice()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:recording',
    dispose: vi.fn(),
  })
})

afterEach(() => vi.unstubAllGlobals())

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
    await screen.findByRole('button', { name: 'Подтянуть слабые темы' }),
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
  expect(screen.getByRole('button', { name: 'Ответить голосом' })).toBeVisible()
  const tutorPanel = screen.getByRole('complementary', { name: 'Репетитор' })
  expect(
    within(tutorPanel).getByRole('button', { name: 'Ответить голосом' }),
  ).toBeVisible()
  expect(
    within(tutorPanel).getByRole('button', { name: 'Пропустить упражнение' }),
  ).toBeVisible()
  expect(
    within(tutorPanel).getByRole('img', { name: 'Визуализация записи' }),
  ).toBeVisible()
  expect(
    within(tutorPanel).queryByText('Нажмите на микрофон, чтобы начать запись'),
  ).not.toBeInTheDocument()
  const workspace = screen.getByRole('main')
  expect(workspace).toHaveAttribute('data-variant', 'training')
  expect(
    screen.getByRole('complementary', { name: 'Репетитор' }),
  ).toContainElement(screen.getByRole('heading', { name: 'Репетитор' }))
  expect(
    screen.getByRole('separator', { name: 'Изменить ширину панели чата' }),
  ).toHaveAttribute('aria-orientation', 'vertical')
  expect(
    screen.queryByRole('heading', { name: 'Цели тренировки' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: 'История ответов' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('textbox', { name: 'Сообщение репетитору' }),
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Ответить голосом' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Остановить и отправить' }),
  )
  expect(
    await screen.findByText(
      'Вы назвали важную часть рассуждения. Какое свойство результата подтверждает ваш выбор?',
    ),
  ).toBeVisible()
  expect(screen.getByText(/Раунд 1/)).toBeVisible()
  expect(screen.queryByText(/Оценка:/)).not.toBeInTheDocument()
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Следующий раунд' }),
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Ответить голосом' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Ответить голосом' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Остановить и отправить' }),
  )
  expect(
    await screen.findByText(
      'Вы связали ответ с условием. Хотите уточнить ещё один признак?',
    ),
  ).toBeVisible()
  expect(screen.queryByText(/Результат проверки:/)).not.toBeInTheDocument()
  expect(screen.queryByText(/2 \/ 2/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Следующий раунд' }))
  expect(
    await screen.findByText(
      'Какой тип задачи предсказывает числовое значение?',
    ),
  ).toBeVisible()
  expect(
    screen.queryByRole('textbox', { name: 'Сообщение репетитору' }),
  ).not.toBeInTheDocument()
  expect(screen.getByText('Главная задача темы')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Пропустить упражнение' }))
  // A failed main task opens theory over the block before the basic tasks.
  expect(
    await screen.findByRole('heading', {
      name: 'Теория по теме «Виды задач машинного обучения»',
    }),
  ).toBeVisible()
  expect(screen.getByText(/Классификация: модель выбирает класс/)).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Пропустить упражнение' }),
  ).not.toBeInTheDocument()
  fireEvent.click(
    screen.getByRole('button', { name: 'Изучил — перейти к задачам' }),
  )
  expect(
    await screen.findByText(
      'Нужно предсказать цену квартиры в рублях. Это классификация или регрессия?',
    ),
  ).toBeVisible()
  expect(screen.getByText('Закрепление 1 из 2')).toBeVisible()
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
