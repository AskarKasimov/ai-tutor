import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { mockApiFetch } from '@/bootstrap/mock-api'
import {
  TrainingApiError,
  type TrainingHistory,
  type TrainingPreview,
  type TrainingProgress,
} from '@/entities/training'
import { userQueryKeys } from '@/entities/user'
import * as audio from '@/shared/lib'
import { TrainingPreview as TrainingPreviewScreen } from '@/pages/trainer/ui/training-preview'

vi.mock('@/shared/lib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/lib')>()),
  createVoiceWaveform: () => ({ setStream: () => {}, dispose: () => {} }),
}))

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
  vi.unstubAllEnvs()
})
function setup(
  entry: {
    existing?: TrainingProgress
    plans?: TrainingPreview[]
    start?: (plan: TrainingPreview) => Promise<TrainingProgress>
    sessionMissing?: boolean
    submit?: () => Promise<TrainingProgress>
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
      async () => plans[Math.min(readPlan.mock.calls.length, plans.length - 1)],
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
  if (entry.sessionMissing)
    readSession.mockRejectedValue(
      new TrainingApiError(404, 'TRAINING_SESSION_NOT_FOUND'),
    )
  else readSession.mockResolvedValue(progress)
  vi.spyOn(dependencies.training, 'submitTraining').mockImplementation(
    async () => (entry.submit ? entry.submit() : progress),
  )
  vi.spyOn(dependencies.training, 'readTrainingHistory').mockImplementation(
    function (...args) {
      const pages = entry.historyPages ?? [{ items: [], targets: [target] }]
      return Promise.resolve(args[1] ? pages[1] : pages[0])
    },
  )
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
      <TrainingPreviewScreen
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
  return { start, readPlan, readSession, onBack }
}

it('renders free preview as untested when the original score is null', async () => {
  setup()
  expect(
    (await screen.findAllByRole('heading', { name: 'Свободная тренировка' }))
      .length,
  ).toBe(1)
  expect(screen.getByText('Исходный результат не проверен.')).toBeVisible()
  expect(screen.queryByText(/Исходный балл: 0/)).not.toBeInTheDocument()
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
    screen.getByRole('button', { name: 'Начать тренировку' }),
  ).toBeDisabled()
  expect(start).not.toHaveBeenCalled()
})

it('refreshes stale preview and waits for a second explicit start click', async () => {
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
  fireEvent.click(
    await screen.findByRole('button', { name: 'Начать тренировку' }),
  )
  expect(await screen.findByText(/План обновился/)).toBeVisible()
  await waitFor(() => expect(readPlan).toHaveBeenCalledTimes(2))
  expect(screen.getByText(/Обновлённая тема/)).toBeVisible()
  expect(start).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Начать тренировку' }))
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(start).toHaveBeenCalledTimes(2)
})

it('offers a resume card and continues an existing session without posting a new start', async () => {
  const { start } = setup({ existing: progress })
  expect(
    await screen.findByRole('button', { name: 'Продолжить тренировку' }),
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: /Вы остановились на раунде/ }),
  ).toBeVisible()
  expect(screen.queryByText(/Исходный балл/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Продолжить тренировку' }))
  expect(await screen.findByText('Вопрос')).toBeVisible()
  expect(start).not.toHaveBeenCalled()
})

it('clears a missing session and returns to the subject selector', async () => {
  const { onBack } = setup({ existing: progress, sessionMissing: true })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить тренировку' }),
  )
  expect(
    await screen.findByText('Текущая тренировка больше недоступна.'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'На главную' }))
  expect(await screen.findByText('Выбор предмета')).toBeVisible()
  expect(onBack).toHaveBeenCalledOnce()
})

it('refreshes the current session after an exercise conflict', async () => {
  const { readSession } = setup({
    existing: progress,
    submit: () =>
      Promise.reject(
        new TrainingApiError(409, 'TRAINING_EXERCISE_NOT_CURRENT'),
      ),
  })
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:voice',
    dispose: vi.fn(),
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить тренировку' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  fireEvent.click(
    await screen.findByRole('button', { name: /Завершить запись/ }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Текущий вопрос изменился.',
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Обновить текущий вопрос' }),
  )
  await waitFor(() => expect(readSession).toHaveBeenCalledTimes(2))
})

it('disposes an active recording when Back returns to the selector', async () => {
  const dispose = vi.fn()
  setup({ existing: progress })
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose,
    stop: vi.fn(),
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить тренировку' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Начать запись' }))
  expect(
    await screen.findByRole('button', { name: /Завершить запись/ }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'На главную' }))
  expect(await screen.findByText('Выбор предмета')).toBeVisible()
  expect(dispose).toHaveBeenCalled()
})

it('loads additional answer history pages on demand', async () => {
  const attempt = (sequence: number, text: string) => ({
    sequence,
    exercise_id: `e${sequence}`,
    round: sequence,
    target_index: 0,
    transcription_id: `t${sequence}`,
    text,
    score: 1,
    max_score: 2,
    verdict: 'partial' as const,
    criterion_results: [],
    feedback: [],
    created_at: sequence,
  })
  setup({
    existing: progress,
    historyPages: [
      {
        items: [attempt(1, 'Первая запись')],
        targets: [target],
        next_cursor: 'next',
      },
      { items: [attempt(2, 'Вторая запись')], targets: [target] },
    ],
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Продолжить тренировку' }),
  )
  expect(await screen.findByText('Первая запись')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Показать ещё' }))
  expect(await screen.findByText('Вторая запись')).toBeVisible()
})
