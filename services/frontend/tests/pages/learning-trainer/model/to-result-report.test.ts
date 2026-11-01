import { expect, it } from 'vitest'
import type {
  DiagnosticAnswer,
  DiagnosticResult,
  DiagnosticTask,
} from '@/entities/learning-diagnostic-session'
import { toResultReport } from '@/pages/learning-trainer/model/to-result-report'

const task = (
  id: string,
  role: DiagnosticTask['role'],
  constituent = 'c1',
): DiagnosticTask => ({
  variant_task_id: id,
  source_task_id: `source-${id}`,
  role,
  competency_id: 'competency',
  competency_name: 'Компетенция',
  constituent_id: constituent,
  constituent_name: 'Составляющая',
  outcome_id: `outcome-${id}`,
  outcome_name: `Результат ${id}`,
  question: 'Вопрос',
  options: [],
  voice_instruction: 'Ответьте',
})

const answer = (
  value: DiagnosticTask,
  score: number,
  skipped = false,
): DiagnosticAnswer => ({
  skipped,
  variant_task_id: value.variant_task_id,
  source_task_id: value.source_task_id,
  competency_id: value.competency_id,
  outcome_id: value.outcome_id,
  role: value.role,
  task: value,
  transcription_id: 'tr',
  text: 'Ответ',
  grader_score: score,
  grader_max_score: value.role === 'main' ? 3 : 1,
  score,
  report_score: value.role === 'main' ? score : score * 3,
  report_max_score: 3,
  verdict: score === 0 ? 'incorrect' : 'correct',
  criterion_results: [],
  feedback: ['Первое.', 'Второе.'],
  created_at: 1,
})

const result = (
  answers: DiagnosticAnswer[],
  untested: DiagnosticTask[] = [],
): DiagnosticResult => ({
  session_id: 's',
  status: 'completed',
  variant_id: 'v',
  map_revision: 1,
  included_competency_count: 1,
  skipped_competencies: [],
  completed_tasks: answers.length,
  total_tasks: answers.length + untested.length,
  diagnostic_score: 3,
  maximum_score: 3,
  report_score: 7,
  report_max_score: 9,
  grade: 'B',
  score_percent: 77,
  answers,
  untested_basics: untested,
})

it('shows a task in points and links a main task to the basics of its constituent', () => {
  const report = toResultReport(
    result([
      answer(task('m', 'main'), 1),
      answer(task('b1', 'basic'), 1),
      answer(task('b2', 'basic'), 0),
      answer(task('other', 'basic', 'c2'), 1),
    ]),
  )
  expect(report).toMatchObject({
    grade: 'B',
    score: 7,
    maxScore: 9,
    percent: 77,
  })
  const [main, full, gap] = report.topics
  expect(main).toMatchObject({
    status: 'partial',
    score: 1,
    maxScore: 3,
    main: true,
    requires: ['b1', 'b2'],
    note: 'Первое. Второе.',
  })
  expect(full).toMatchObject({
    status: 'full',
    score: 3,
    maxScore: 3,
    main: false,
    requires: [],
  })
  expect(gap).toMatchObject({ status: 'gap', score: 0 })
})

it('credits the basics that a full main answer left out', () => {
  const report = toResultReport(
    result([answer(task('m', 'main'), 3)], [task('b1', 'basic')]),
  )
  expect(report.topics[0]).toMatchObject({ status: 'full', requires: ['b1'] })
  expect(report.topics[1]).toMatchObject({
    id: 'b1',
    status: 'credited',
    score: null,
  })
})

it('marks a skipped task as a gap', () => {
  const [topic] = toResultReport(
    result([answer(task('m', 'main'), 0, true)]),
  ).topics
  expect(topic).toMatchObject({ status: 'gap', skipped: true })
})

it('leaves out the line sending the student on to the next task', () => {
  const [topic] = toResultReport(
    result([
      {
        ...answer(task('m', 'main'), 0, true),
        feedback: [
          'Вопрос пропущен.',
          'Ответ оценён в 0 баллов.',
          'Продолжите со следующим вопросом.',
        ],
      },
    ]),
  ).topics
  expect(topic.note).toBe('Вопрос пропущен. Ответ оценён в 0 баллов.')
})

it('keeps the reasons of the criteria without inventing points', () => {
  const asked = answer(task('m', 'main'), 1)
  const { topics } = toResultReport({
    ...result([asked]),
    answers: [
      {
        ...asked,
        criterion_results: [
          { key: 'cause', satisfied: false, explanation: 'Нет причины.' },
        ],
      },
    ],
  })
  expect(topics[0].marks).toEqual([
    {
      id: 'cause',
      title: '',
      met: false,
      reason: 'Нет причины.',
      quote: '',
    },
  ])
})
