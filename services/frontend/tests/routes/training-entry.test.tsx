import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { mockApiFetch } from '@/bootstrap/mock-api'
import {
  TrainingApiError,
  type TrainingHistory,
  type TrainingChatPage,
  type TrainingPreview,
  type TrainingProgress,
} from '@/entities/training'
import { userQueryKeys } from '@/entities/user'
import { TrainingEntry } from '@/pages/trainer/ui/training-entry'
import { mockChatVoice } from '../support/mock-chat-voice'

const target = {
  kind: 'partial_competency',
  label: 'Частичные пробелы в знаниях',
  competency_id: 'c1',
  competency_name: 'Тема',
  outcome_id: 'o1',
  outcome_name: 'Результат',
  original_score: null,
  original_max_score: 2,
  last_score: null,
}
const progress: TrainingProgress = {
  session_id: 't1',
  diagnostic_session_id: 'd1',
  subject_id: 's1',
  subject_name: 'Предмет',
  mode: 'focused',
  status: 'active',
  round: 3,
  answer_count: 7,
  targets: [target],
  current: {
    exercise_id: 'e1',
    outcome_id: 'o1',
    outcome_name: 'Результат',
    question: 'Вопрос',
    options: ['А', 'Б'],
    voice_instruction: 'Ответьте',
  },
}
const preview = (
  overrides: Partial<TrainingPreview> = {},
): TrainingPreview => ({
  diagnostic_session_id: 'd1',
  subject_id: 's1',
  subject_name: 'Предмет',
  mode: 'free_practice',
  plan_revision: 1,
  diagnostic_score: 2,
  maximum_score: 2,
  status: 'ready',
  confirmed_gaps: [],
  partial_competencies: [],
  topics: [target],
  ...overrides,
})

beforeEach(async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  await mockApiFetch('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email: '', password: '' }),
  })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})
function setup(
  entry: {
    existing?: TrainingProgress
    plans?: TrainingPreview[]
    start?: (plan: TrainingPreview) => Promise<TrainingProgress>
    sessionMissing?: boolean
    submit?: () => Promise<TrainingProgress>
    sendChat?: () => Promise<never>
    historyPages?: TrainingHistory[]
  } = {},
) {
  const dependencies = createAppDependencies()
  vi.spyOn(
    dependencies.training,
    'findTrainingForDiagnostic',
  ).mockImplementation(async () => {
    if (entry.existing) return entry.existing
    throw new TrainingApiError(404, 'TRAINING_NOT_FOUND')
  })
  const plans = entry.plans ?? [preview()]
  const readPlan = vi
    .spyOn(dependencies.training, 'readTrainingPreview')
    .mockImplementation(
      async () =>
        plans[Math.min(readPlan.mock.calls.length - 1, plans.length - 1)],
    )
  const start = vi
    .spyOn(dependencies.training, 'startTraining')
    .mockImplementation(async () =>
      entry.start
        ? entry.start(
            plans[Math.min(start.mock.calls.length - 1, plans.length - 1)],
          )
        : progress,
    )
  const readSession = vi.spyOn(dependencies.training, 'readTrainingSession')
  const readChat = vi.spyOn(dependencies.training, 'readTrainingChat')
  readChat.mockResolvedValue({
    session_id: progress.session_id,
    exercise_id: progress.current.exercise_id,
    revision: 0,
    status: 'active',
    items: [
      {
        id: 'welcome',
        sequence: 1,
        role: 'assistant',
        kind: 'welcome',
        text: 'Начнём разбирать задачу.',
        created_at: 1,
      },
    ],
    next_cursor: null,
    latest_check: null,
    pending_turn: null,
  } satisfies TrainingChatPage)
  if (entry.sessionMissing)
    readSession.mockRejectedValue(
      new TrainingApiError(404, 'TRAINING_SESSION_NOT_FOUND'),
    )
  else readSession.mockResolvedValue(progress)
  vi.spyOn(dependencies.training, 'submitTraining').mockImplementation(
    async () => (entry.submit ? entry.submit() : progress),
  )
  if (entry.sendChat)
    vi.spyOn(dependencies.training, 'sendTrainingChatVoice').mockImplementation(
      async () => entry.sendChat!(),
    )
  const readHistory = vi
    .spyOn(dependencies.training, 'readTrainingHistory')
    .mockImplementation(function (...args) {
      const pages = entry.historyPages ?? [{ items: [], targets: [target] }]
      return Promise.resolve(args[1] ? pages[1] : pages[0])
    })
  const onBack = vi.fn()
  const queryClient = createQueryClient()
  queryClient.setQueryData(userQueryKeys.auth, {
    id: 'demo-student',
    email: 'student@example.com',
    display_name: 'Demo',
    role: 'student',
    created_at: 0,
  })
  function Harness() {
    const [visible, setVisible] = useState(true)
    return visible ? (
      <TrainingEntry
        userId="demo-student"
        diagnosticId="d1"
        subjectName="Предмет"
        onBack={() => {
          onBack()
          setVisible(false)
        }}
      />
    ) : (
      <div>Выбор предмета</div>
    )
  }
  render(
    <AppProviders queryClient={queryClient} dependencies={dependencies}>
      <Harness />
    </AppProviders>,
  )
  return { start, readPlan, readSession, readChat, readHistory, onBack }
}

it('opens a new training directly without an extra start click', async () => {
  const { start } = setup()
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Начать тренировку' }),
  ).not.toBeInTheDocument()
  expect(start).toHaveBeenCalledOnce()
  expect(start).toHaveBeenCalledWith(
    'd1',
    expect.any(String),
    1,
    expect.any(AbortSignal),
  )
})

it('does not allow a start when the preview has no practice tasks', async () => {
  const { start } = setup({
    plans: [preview({ status: 'no_practice_tasks', topics: [] })],
  })
  expect(
    await screen.findByText(
      'Для свободной тренировки пока нет подходящих заданий.',
    ),
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Начать тренировку' }),
  ).not.toBeInTheDocument()
  expect(start).not.toHaveBeenCalled()
})

it('refreshes a stale plan and allows retrying the failed start', async () => {
  const refreshed = preview({
    plan_revision: 2,
    topics: [{ ...target, outcome_name: 'Обновлённая тема' }],
  })
  const { start, readPlan } = setup({
    plans: [preview(), refreshed],
    start: vi
      .fn()
      .mockRejectedValueOnce(
        new TrainingApiError(409, 'TRAINING_PREVIEW_STALE'),
      )
      .mockResolvedValue(progress),
  })
  expect(await screen.findByText(/План обновился/)).toBeVisible()
  await waitFor(() => expect(readPlan).toHaveBeenCalledTimes(2))
  expect(start).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(start).toHaveBeenCalledTimes(2)
})

it('opens an existing session directly without posting a new start', async () => {
  const { start } = setup({ existing: progress })
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Продолжить тренировку' }),
  ).not.toBeInTheDocument()
  expect(start).not.toHaveBeenCalled()
})

it('allows retrying a failed automatic start', async () => {
  const { start } = setup({
    start: vi
      .fn()
      .mockRejectedValueOnce(new TrainingApiError(500, 'ERROR'))
      .mockResolvedValue(progress),
  })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось начать тренировку',
  )
  expect(start).toHaveBeenCalledOnce()
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(start).toHaveBeenCalledTimes(2)
  expect(start.mock.calls[1][1]).toBe(start.mock.calls[0][1])
})

it('clears a missing session and returns to the subject selector', async () => {
  const { onBack } = setup({ existing: progress, sessionMissing: true })
  expect(
    await screen.findByText('Текущая тренировка больше недоступна.'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'На главную' }))
  expect(await screen.findByText('Выбор предмета')).toBeVisible()
  expect(onBack).toHaveBeenCalledOnce()
})

it('refetches chat after a voice revision conflict', async () => {
  const { readChat } = setup({
    existing: progress,
    sendChat: () =>
      Promise.reject(
        new TrainingApiError(409, 'TRAINING_EXERCISE_NOT_CURRENT'),
      ),
  })
  mockChatVoice()
  await screen.findByText('Начнём разбирать задачу.')
  fireEvent.click(
    await screen.findByRole('button', { name: 'Ответить голосом' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Остановить и отправить' }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Состояние диалога изменилось. Обновляем историю…',
  )
  await waitFor(() => expect(readChat).toHaveBeenCalledTimes(2))
})

it('returns to the selector from the chat session', async () => {
  setup({ existing: progress })
  expect(
    await screen.findByRole('heading', { name: 'Репетитор' }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'На главную' }))
  expect(await screen.findByText('Выбор предмета')).toBeVisible()
})

it('does not render training targets or fetch answer history', async () => {
  const { readHistory } = setup({ existing: progress })
  expect(
    await screen.findByRole('heading', { name: 'Репетитор' }),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Цели тренировки' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: 'История ответов' }),
  ).not.toBeInTheDocument()
  expect(readHistory).not.toHaveBeenCalled()
})

it('offers one voice action without a separate check mode', async () => {
  setup({ existing: progress })
  expect(
    await screen.findByRole('button', { name: 'Ответить голосом' }),
  ).toBeVisible()
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Записать итоговый ответ' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('textbox', { name: 'Сообщение репетитору' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Отправить' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Нужна подсказка' }),
  ).not.toBeInTheDocument()
})
