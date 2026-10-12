import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import type { LearningMessage, LearningPlan } from '@/entities/learning-plan'
import { userQueryKeys } from '@/entities/user'
import { LearningPlanEntry } from '@/pages/trainer/ui/learning-plan'
import { mockChatVoice } from '../support/mock-chat-voice'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const item = (index: number, overrides = {}) => ({
  index,
  competency_id: 'c1',
  competency_name: 'Регрессия',
  outcome_id: `o${index}`,
  outcome_name: `Результат ${index + 1}`,
  theory_minutes: 15,
  practice_minutes: 15,
  in_plan: true,
  status: 'pending' as const,
  attempts: 0,
  ...overrides,
})
const plan = (overrides: Partial<LearningPlan> = {}): LearningPlan => ({
  id: 'p1',
  diagnostic_session_id: 'd1',
  subject_id: 's1',
  subject_name: 'ML',
  hours: 1,
  detail: 'brief',
  status: 'active',
  current_index: 0,
  progress_percent: 0,
  planned_minutes: 30,
  pending_answer: null,
  items: [
    item(0, { status: 'current' }),
    item(1),
    item(2, { in_plan: false, outcome_name: 'Сверх плана' }),
  ],
  ...overrides,
})
const lesson: LearningMessage[] = [
  {
    sequence: 1,
    item_index: 0,
    role: 'assistant',
    kind: 'theory',
    text: 'Теория по теме',
    created_at: 1,
  },
  {
    sequence: 2,
    item_index: 0,
    role: 'assistant',
    kind: 'practice',
    text: 'Практический вопрос?',
    created_at: 1,
  },
]

function setup(
  existing: LearningPlan | null,
  configure?: (
    api: ReturnType<typeof createAppDependencies>['learningPlan'],
  ) => void,
) {
  const dependencies = createAppDependencies()
  const api = dependencies.learningPlan
  vi.spyOn(api, 'findLearningPlan').mockResolvedValue(existing)
  const create = vi.spyOn(api, 'createLearningPlan').mockResolvedValue(plan())
  let opened = false
  vi.spyOn(api, 'readLearningMessages').mockImplementation(
    async (_id, index) => (index === 0 && opened ? lesson : []),
  )
  let current = existing ?? plan()
  const open = vi.spyOn(api, 'openLesson').mockImplementation(async () => {
    opened = true
    return current
  })
  const answer = vi.spyOn(api, 'answerLesson').mockImplementation(async () => {
    current = plan({
      current_index: 1,
      progress_percent: 50,
      items: [
        item(0, { status: 'passed' }),
        item(1, { status: 'current' }),
        item(2, { in_plan: false, outcome_name: 'Сверх плана' }),
      ],
    })
    return current
  })
  const queryClient = createQueryClient()
  queryClient.setQueryData(userQueryKeys.auth, {
    id: 'u1',
    email: 'student@example.edu',
    display_name: 'Студент',
    role: 'student',
    created_at: 0,
  })
  configure?.(api)
  render(
    <AppProviders queryClient={queryClient} dependencies={dependencies}>
      <LearningPlanEntry
        userId="u1"
        diagnosticId="d1"
        subjectName="ML"
        onBack={() => {}}
      />
    </AppProviders>,
  )
  return { create, open, answer }
}

it('asks for the hours once and builds the plan with one click', async () => {
  const { create } = setup(null)
  expect(
    await screen.findByRole('heading', { name: 'Сколько у тебя часов?' }),
  ).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /3 часа/ }))
  await waitFor(() =>
    expect(create).toHaveBeenCalledWith('d1', 3, expect.anything()),
  )
  expect(
    await screen.findByRole('navigation', { name: 'Оглавление' }),
  ).toBeInTheDocument()
})

it('teaches the current topic in the chat and moves on after a voice answer', async () => {
  mockChatVoice()
  const { open, answer } = setup(plan())
  expect(await screen.findByText('Теория по теме')).toBeInTheDocument()
  expect(open).toHaveBeenCalledTimes(1)
  expect(screen.getByText('Практический вопрос?')).toBeInTheDocument()
  expect(screen.getByText('Если останется время')).toBeInTheDocument()
  // Future topics stay closed.
  expect(screen.getByRole('button', { name: /Результат 2/ })).toBeDisabled()
  expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')

  fireEvent.click(await screen.findByRole('button', { name: 'Записать ответ' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  await waitFor(() => expect(answer).toHaveBeenCalledTimes(1))
  // A correct answer is celebrated instead of jumping to the next topic; the
  // next lesson is prepared meanwhile.
  const dialog = await screen.findByRole('dialog', { name: 'Еее, ты молодец!' })
  expect(dialog).toHaveTextContent('Тема «Результат 1» освоена')
  expect(dialog).toHaveTextContent('С первой попытки!')
  expect(
    screen.getByRole('heading', { level: 2, name: 'Результат 1' }),
  ).toBeVisible()
  await waitFor(() => expect(open).toHaveBeenCalledTimes(2))
  fireEvent.click(screen.getByRole('button', { name: /Продолжить/ }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(
    screen.getByRole('heading', { level: 2, name: 'Результат 2' }),
  ).toBeVisible()
  expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '50')
  // The passed topic opens read-only.
  fireEvent.click(screen.getByRole('button', { name: /Результат 1/ }))
  await waitFor(() =>
    expect(
      screen.getByText('История темы — только просмотр.'),
    ).toBeInTheDocument(),
  )
  expect(screen.queryByRole('button', { name: 'Записать ответ' })).toBeNull()
})

it('shows the finished plan with topics to review', async () => {
  setup(
    plan({
      status: 'completed',
      current_index: null,
      progress_percent: 100,
      items: [item(0, { status: 'passed' }), item(1, { status: 'review' })],
    }),
  )
  expect(await screen.findByText('План пройден!')).toBeInTheDocument()
  expect(screen.getByText(/1 тему стоит повторить/)).toBeInTheDocument()
})

it('shows theory while practice is pending and enables recording after it arrives', async () => {
  let theoryReady = false
  let practiceReady = false
  let finishOpen!: (value: LearningPlan) => void
  const pendingOpen = new Promise<LearningPlan>((resolve) => {
    finishOpen = resolve
  })
  const { open } = setup(plan(), (api) => {
    vi.mocked(api.readLearningMessages).mockImplementation(async () =>
      practiceReady ? lesson : theoryReady ? lesson.slice(0, 1) : [],
    )
    vi.mocked(api.openLesson).mockReturnValue(pendingOpen)
  })
  await waitFor(() => expect(open).toHaveBeenCalledTimes(1))
  theoryReady = true
  await waitFor(
    () => expect(screen.getByText('Теория по теме')).toBeVisible(),
    { timeout: 4000 },
  )
  expect(
    screen.getByText('Практика готовится — пока можно прочитать теорию.'),
  ).toBeVisible()
  expect(screen.queryByText('Практический вопрос?')).toBeNull()
  expect(screen.getByRole('button', { name: 'Записать ответ' })).toBeDisabled()
  practiceReady = true
  finishOpen(plan())
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  expect(screen.getByText('Теория по теме')).toBeVisible()
  expect(
    screen.queryByText('Практика готовится — пока можно прочитать теорию.'),
  ).toBeNull()
})

it('keeps theory after a practice failure and lets the student retry', async () => {
  let theoryReady = false
  let practiceReady = false
  const { open } = setup(plan(), (api) => {
    vi.mocked(api.readLearningMessages).mockImplementation(async () =>
      practiceReady ? lesson : theoryReady ? lesson.slice(0, 1) : [],
    )
    vi.mocked(api.openLesson)
      .mockImplementationOnce(async () => {
        theoryReady = true
        throw new Error('practice unavailable')
      })
      .mockImplementation(async () => {
        practiceReady = true
        return plan()
      })
  })
  expect(await screen.findByText('Теория по теме')).toBeVisible()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Наставник не ответил. Попробуйте ещё раз.',
  )
  expect(screen.getByRole('button', { name: 'Записать ответ' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  expect(open).toHaveBeenCalledTimes(2)
  expect(screen.getAllByText('Теория по теме')).toHaveLength(1)
})

it('resumes an incomplete lesson after reopening the page', async () => {
  let practiceReady = false
  const { open } = setup(plan(), (api) => {
    vi.mocked(api.readLearningMessages).mockImplementation(async () =>
      practiceReady ? lesson : lesson.slice(0, 1),
    )
    vi.mocked(api.openLesson).mockImplementation(async () => {
      practiceReady = true
      return plan()
    })
  })
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  expect(open).toHaveBeenCalledTimes(1)
  expect(screen.getAllByText('Теория по теме')).toHaveLength(1)
})
