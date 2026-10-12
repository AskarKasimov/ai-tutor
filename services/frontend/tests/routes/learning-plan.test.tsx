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

function setup(existing: LearningPlan | null, messages = lesson) {
  const dependencies = createAppDependencies()
  const api = dependencies.learningPlan
  vi.spyOn(api, 'findLearningPlan').mockResolvedValue(existing)
  const create = vi.spyOn(api, 'createLearningPlan').mockResolvedValue(plan())
  let opened = false
  vi.spyOn(api, 'readLearningMessages').mockImplementation(
    async (_id, index) => (index === 0 && opened ? messages : []),
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
  const { container } = render(
    <AppProviders queryClient={queryClient} dependencies={dependencies}>
      <LearningPlanEntry
        userId="u1"
        diagnosticId="d1"
        subjectName="ML"
        onBack={() => {}}
      />
    </AppProviders>,
  )
  return { create, open, answer, container }
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

it('renders teacher Markdown while preserving code and literal student answers', async () => {
  const theory = [
    '# Обновление весов',
    '**Градиент** задаёт направление.',
    '',
    '- Вычислите ошибку.',
    '- Обновите веса.',
    '',
    '| Шаг | Loss |',
    '|---|---:|',
    '| 1 | 0.5 |',
    '',
    '```python',
    'for step in range(2):',
    '    print("a\\nb")',
    '```',
  ].join('\n')
  const { container } = setup(plan(), [
    { ...lesson[0], text: theory },
    { ...lesson[1], text: 'Объясните `grad_w`.' },
    {
      ...lesson[0],
      sequence: 3,
      kind: 'feedback',
      text: '**Верно:** уменьшайте loss.',
    },
    {
      ...lesson[0],
      sequence: 4,
      role: 'user',
      kind: 'answer',
      text: '**Это мои слова**',
    },
  ])
  await screen.findByText('Теория')
  expect(screen.getByRole('table')).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Обновление весов', level: 3 }),
  ).toBeVisible()
  expect(screen.getByText('Градиент').tagName).toBe('STRONG')
  expect(screen.getByText('grad_w').tagName).toBe('CODE')
  expect(screen.getByText('Верно:').tagName).toBe('STRONG')
  expect(container.querySelector('pre')?.textContent).toBe(
    'for step in range(2):\n    print("a\\nb")\n',
  )
  expect(screen.getByText('**Это мои слова**')).toBeVisible()
})
