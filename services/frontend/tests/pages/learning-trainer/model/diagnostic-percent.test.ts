import { expect, it } from 'vitest'
import type { DiagnosticProgress } from '@/entities/learning-diagnostic-session'
import { diagnosticPercent } from '@/pages/learning-trainer/model/diagnostic-percent'

const base: DiagnosticProgress = {
  session_id: 's',
  status: 'active',
  completed_tasks: 0,
  skipped_tasks: 0,
  total_tasks: 30,
  competency_count: 10,
  current_competency: 1,
  current_step: 0,
}
const at = (competency: number, step = 0): DiagnosticProgress => ({
  ...base,
  current_competency: competency,
  current_step: step,
})

it('closes a whole topic share after a good main answer', () => {
  expect(diagnosticPercent(at(1))).toBe(0)
  expect(diagnosticPercent(at(2))).toBe(10)
  expect(diagnosticPercent(at(10))).toBe(90)
  expect(diagnosticPercent({ ...at(10), status: 'completed' })).toBe(100)
})

it('grows by a third of the topic for a weak main and each basic', () => {
  // 90% before the last topic: the weak main and its basics never go back.
  expect(
    [at(10), at(10, 1), at(10, 2)].map((progress) =>
      diagnosticPercent(progress),
    ),
  ).toEqual([90, 93, 97])
  expect(diagnosticPercent({ ...at(10, 2), status: 'completed' })).toBe(100)
})

it('never decreases and changes after every task of a mixed run', () => {
  // Topic 1 weak (main + 2 basics), topic 2 good, topic 3 weak with one basic.
  const run = [at(1), at(1, 1), at(1, 2), at(2), at(3), at(3, 1), at(4)]
  const values = run.map((progress) => diagnosticPercent(progress))
  for (let i = 1; i < values.length; i++)
    expect(values[i]).toBeGreaterThan(values[i - 1])
})

it('stays below 100 while the diagnostic is running', () => {
  expect(
    diagnosticPercent({ ...at(1, 2), competency_count: 1, total_tasks: 3 }),
  ).toBe(67)
  expect(
    diagnosticPercent({ ...at(300, 2), competency_count: 300 }),
  ).toBeLessThan(100)
})

it('falls back to answered tasks for sessions without topics', () => {
  expect(
    diagnosticPercent({
      ...base,
      competency_count: undefined,
      current_competency: undefined,
      current_step: undefined,
      completed_tasks: 2,
      skipped_tasks: 1,
      total_tasks: 6,
    }),
  ).toBe(50)
})
