import { expect, it } from 'vitest'
import type {
  DiagnosticResult,
  TopicResult,
} from '@/entities/diagnostic-session'
import { toResultReport } from '@/pages/trainer/lib/to-result-report'

const topic = (overrides: Partial<TopicResult>): TopicResult => ({
  nodeId: 'a',
  title: 'Тема',
  status: 'independent',
  grade: 'A',
  topLevel: true,
  requires: [],
  assisted: false,
  score: 4,
  maxScore: 4,
  marks: [],
  reply: '',
  ...overrides,
})

const result = (topics: TopicResult[]): DiagnosticResult => ({
  sessionId: 's',
  grade: 'B',
  scorePercent: 75,
  score: 3,
  maxScore: 4,
  topics,
  gaps: [],
  weakSpots: [],
})

const statusOf = (overrides: Partial<TopicResult>) =>
  toResultReport(result([topic(overrides)])).topics[0].status

it('shows an asked topic by its points, not its status', () => {
  expect(statusOf({ score: 4, maxScore: 4 })).toBe('full')
  expect(statusOf({ score: 3, maxScore: 4, status: 'assisted' })).toBe(
    'partial',
  )
  expect(statusOf({ score: 4, maxScore: 4, status: 'assisted' })).toBe('full')
  // Not a gap, so at least partial: no points never read as a full answer.
  expect(statusOf({ score: 0, maxScore: 1 })).toBe('partial')
})

it('keeps the statuses that points cannot express', () => {
  expect(statusOf({ status: 'gap', score: 1, maxScore: 4 })).toBe('gap')
  expect(statusOf({ status: 'credited_by_parent', score: null })).toBe(
    'credited',
  )
  expect(statusOf({ status: 'insufficient', score: null })).toBe('insufficient')
  expect(statusOf({ status: 'unchecked', score: null })).toBe('unchecked')
})

it('passes the overall points, the closing reply and the marks', () => {
  const marks = [
    { id: 'r1', title: 'Суть', score: 1, maxScore: 2, reason: 'р', quote: 'ц' },
  ]
  const report = toResultReport(
    result([topic({ reply: 'Разбор', marks, requires: ['b'] })]),
  )
  expect(report).toMatchObject({
    grade: 'B',
    score: 3,
    maxScore: 4,
    percent: 75,
  })
  expect(report.topics[0]).toMatchObject({
    note: 'Разбор',
    marks,
    requires: ['b'],
    main: true,
  })
})
