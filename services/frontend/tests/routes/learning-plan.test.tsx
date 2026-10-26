import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import type { LearningMessage, LearningPlan } from '@/entities/learning-plan'
import { replaceSession } from '@/entities/user'
import { LearningPlanEntry } from '@/pages/learning-trainer/ui/learning-plan'
import { mockChatVoice } from '../support/mock-chat-voice'
import * as browserAudio from '@/shared/lib'

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

async function setup(
  existing: LearningPlan | null,
  configure?: (
    api: ReturnType<typeof createAppDependencies>['learningPlan'],
  ) => void,
  noGaps = false,
  messages = lesson,
  failFirstMessageRead = false,
) {
  const dependencies = createAppDependencies()
  vi.spyOn(
    dependencies.learningDiagnostic,
    'readDiagnosticResult',
  ).mockResolvedValue({
    session_id: 'd1',
    status: 'completed',
    variant_id: 'v1',
    map_revision: 1,
    included_competency_count: 1,
    skipped_competencies: [],
    completed_tasks: 1,
    total_tasks: 3,
    diagnostic_score: noGaps ? 2 : 1,
    maximum_score: 2,
    untested_basics: [],
    answers: [
      {
        variant_task_id: 't1',
        source_task_id: 't1',
        competency_id: 'c1',
        outcome_id: 'o1',
        role: 'main',
        task: {
          variant_task_id: 't1',
          source_task_id: 't1',
          role: 'main',
          competency_id: 'c1',
          competency_name: 'ML',
          constituent_id: 'part1',
          constituent_name: 'Тема',
          outcome_id: 'o1',
          outcome_name: 'Результат',
          question: 'Вопрос',
          options: [],
          voice_instruction: 'Ответьте',
        },
        transcription_id: 'tr1',
        text: 'Ответ',
        grader_score: noGaps ? 2 : 1,
        grader_max_score: 2,
        score: noGaps ? 2 : 1,
        verdict: noGaps ? 'correct' : 'partial',
        criterion_results: [],
        feedback: [],
        created_at: 1,
      },
    ],
  })
  const api = dependencies.learningPlan
  vi.spyOn(api, 'previewAnswer').mockResolvedValue({
    id: 'preview-id',
    text: 'Предварительная расшифровка',
  })
  vi.spyOn(api, 'findLearningPlan').mockResolvedValue(existing)
  const create = vi.spyOn(api, 'createLearningPlan').mockResolvedValue(plan())
  let opened = false
  let firstMessageRead = true
  vi.spyOn(api, 'readLearningMessages').mockImplementation(
    async (_id, index) => {
      if (failFirstMessageRead && firstMessageRead) {
        firstMessageRead = false
        throw new Error('temporary history failure')
      }
      return index === 0 && opened ? messages : []
    },
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
  await replaceSession(queryClient, {
    id: 'u1',
    email: 'student@example.edu',
    display_name: 'Студент',
    role: 'student',
  })
  configure?.(api)
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
  const { create } = await setup(null)
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

it('does not ask for hours or create a plan when checked answers have no gaps', async () => {
  const { create } = await setup(null, undefined, true)
  expect(
    await screen.findByText(
      'По проверенным заданиям пробелов не выявлено. Дополнительная подготовка не требуется.',
    ),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Сколько у тебя часов?' }),
  ).not.toBeInTheDocument()
  expect(create).not.toHaveBeenCalled()
})

it('teaches the current topic in the chat and moves on after a voice answer', async () => {
  mockChatVoice()
  vi.spyOn(browserAudio, 'createAudioUrl').mockReturnValue({
    url: 'blob:preview',
    dispose: vi.fn(),
  })
  const { open, answer } = await setup(plan())
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
  expect(
    await screen.findByRole('button', { name: 'Отправить ответ' }),
  ).toBeVisible()
  expect(answer).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ответ' }))
  await waitFor(() => expect(answer).toHaveBeenCalledTimes(1))
  // A correct answer is celebrated instead of jumping to the next topic; the
  // next lesson is prepared meanwhile.
  const dialog = await screen.findByRole('dialog', { name: 'Еее, ты молодец!' })
  expect(dialog).toHaveTextContent('Тема «Результат 1» освоена')
  expect(dialog).toHaveTextContent('С первой попытки!')
  expect(screen.getByRole('region', { name: 'Результат 1' })).toBeVisible()
  await waitFor(() => expect(open).toHaveBeenCalledTimes(2))
  fireEvent.click(screen.getByRole('button', { name: /Продолжить/ }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('region', { name: 'Результат 2' })).toBeVisible()
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

it('shows a history error and resumes opening the lesson after retry', async () => {
  const { open } = await setup(plan(), undefined, false, lesson, true)

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось загрузить историю темы.',
  )
  expect(open).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))

  expect(await screen.findByText('Теория по теме')).toBeInTheDocument()
  expect(await screen.findByText('Практический вопрос?')).toBeInTheDocument()
  await waitFor(() => expect(open).toHaveBeenCalledTimes(1))
})

it('shows the finished plan with topics to review', async () => {
  await setup(
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
  const { open } = await setup(plan(), (api) => {
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
})

it('keeps theory after a practice failure and lets the student retry', async () => {
  let theoryReady = false
  let practiceReady = false
  const { open } = await setup(plan(), (api) => {
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
  const { open } = await setup(plan(), (api) => {
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

it('lets the learner replace a recording before explicitly sending it', async () => {
  const media = mockChatVoice()
  const dispose = vi.fn()
  vi.spyOn(browserAudio, 'createAudioUrl').mockReturnValue({
    url: 'blob:preview',
    dispose,
  })
  const { answer } = await setup(plan())
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Записать ответ' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  expect(
    await screen.findByLabelText('Расшифровка перед отправкой'),
  ).toBeVisible()
  expect(answer).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Перезаписать ответ' }))
  expect(browserAudio.createAudioUrl).not.toHaveBeenCalled()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  expect(
    await screen.findByRole('button', { name: 'Отправить ответ' }),
  ).toBeEnabled()
  expect(media.getUserMedia).toHaveBeenCalledTimes(2)
  expect(answer).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ответ' }))
  await waitFor(() => expect(answer).toHaveBeenCalledOnce())
})

it('reviews a recognized answer in the voice bar and sends it without a transcribing status', async () => {
  mockChatVoice()
  const { answer } = await setup(plan(), (api) => {
    vi.mocked(api.answerLesson).mockReturnValue(new Promise(() => {}))
  })
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Записать ответ' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  const transcript = await screen.findByLabelText('Расшифровка перед отправкой')
  expect(transcript).toHaveTextContent('Предварительная расшифровка')
  const remove = screen.getByRole('button', { name: 'Перезаписать ответ' })
  const send = screen.getByRole('button', { name: 'Отправить ответ' })
  // The transcript sits above the bar: delete on the left, send on the right.
  expect(
    transcript.compareDocumentPosition(remove) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy()
  expect(
    remove.compareDocumentPosition(send) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Записать ответ' })).toBeNull()

  fireEvent.click(send)
  await waitFor(() => expect(answer).toHaveBeenCalledOnce())
  expect(screen.queryByText('Распознаём ответ…')).not.toBeInTheDocument()
  expect(
    screen.queryByLabelText('Расшифровка перед отправкой'),
  ).not.toBeInTheDocument()
})

it('clears failed recognition when a new answer is recorded with the microphone', async () => {
  mockChatVoice()
  let preview!: ReturnType<typeof vi.spyOn>
  const { answer } = await setup(plan(), (api) => {
    preview = vi
      .mocked(api.previewAnswer)
      .mockRejectedValueOnce(new Error('STT unavailable'))
      .mockResolvedValue({ id: 'new-transcript', text: 'Новый ответ' })
  })
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Записать ответ' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  const retry = await screen.findByRole('button', {
    name: 'Повторить распознавание',
  })
  expect(retry).toBeEnabled()

  fireEvent.click(screen.getByRole('button', { name: 'Записать ответ' }))
  await screen.findByRole('button', { name: 'Закончить запись' })
  expect(
    screen.queryByRole('button', { name: 'Повторить распознавание' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Перезаписать ответ' }),
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Закончить запись' }))
  expect(
    await screen.findByLabelText('Расшифровка перед отправкой'),
  ).toHaveTextContent('Новый ответ')
  expect(preview).toHaveBeenCalledTimes(2)
  expect(preview.mock.calls[1][0]).not.toBe(preview.mock.calls[0][0])
  fireEvent.click(screen.getByRole('button', { name: 'Отправить ответ' }))
  await waitFor(() =>
    expect(answer).toHaveBeenCalledWith(
      'p1',
      {
        transcription_id: 'new-transcript',
        item_index: 0,
        attempts: 0,
        question: 'Практический вопрос?',
      },
      expect.anything(),
    ),
  )
})

it('drops the local recording once the server owns a transcript awaiting a check retry', async () => {
  mockChatVoice()
  vi.spyOn(browserAudio, 'createAudioUrl').mockReturnValue({
    url: 'blob:preview',
    dispose: vi.fn(),
  })
  let pending = false
  await setup(plan(), (api) => {
    vi.mocked(api.findLearningPlan).mockImplementation(async () =>
      plan({ pending_answer: pending ? 'Распознанный ответ' : null }),
    )
    vi.mocked(api.answerLesson).mockImplementation(async () => {
      pending = true
      throw new Error('check failed')
    })
    vi.spyOn(api, 'retryLessonAnswer').mockImplementation(async () => {
      pending = false
      return plan()
    })
  })
  expect(await screen.findByText('Практический вопрос?')).toBeVisible()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Записать ответ' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Закончить запись' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Отправить ответ' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Проверить ещё раз' }),
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Записать ответ' }),
    ).toBeEnabled(),
  )
  expect(
    screen.queryByRole('button', { name: 'Отправить ответ' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByLabelText('Прослушать записанный ответ'),
  ).not.toBeInTheDocument()
})

it('renders teacher Markdown while preserving code and literal student answers', async () => {
  const theory = [
    '# Обновление весов',
    '**Градиент** задаёт направление.',
    'Среднее: $\\frac{a+b}{2}$.',
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
  const { container } = await setup(plan(), undefined, false, [
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
      text: '**Это мои слова** $x^2$',
    },
  ])
  expect(await screen.findAllByText('Теория')).toHaveLength(2)
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
  expect(container.querySelector('math mfrac')).not.toBeNull()
  expect(screen.getByText('**Это мои слова** $x^2$')).toBeVisible()
})
