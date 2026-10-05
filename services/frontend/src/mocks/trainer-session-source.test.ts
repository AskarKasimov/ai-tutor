import { expect, it } from 'vitest'
import { createMockTrainerSessionSource } from './trainer-session-source'

const answer = (assignmentId: string, score: 0 | 1 | 2 = 2) => ({ assignmentId, transcript: 'Мой ответ', assessment: { score, feedback: ['Верно', 'Причина', 'Совет'] as [string, string, string] }, submittedAt: 1791200000 })

it('starts seven unique assignments without prefilled answers and isolates users', async () => {
  const source = createMockTrainerSessionSource()
  const session = await source.getSession('one')
  expect(session.tasks).toHaveLength(7)
  expect(new Set(session.tasks.map((task) => task.taskId)).size).toBe(7)
  expect(session.answers).toEqual([])
  expect(session.currentAssignmentId).toBe(session.tasks[0].assignmentId)
  await source.saveAnswer('one', session.id, answer(session.currentAssignmentId!))
  expect((await source.getSession('two')).answers).toEqual([])
})

it('rejects skipped, repeated and obsolete answers without advancing progress', async () => {
  const source = createMockTrainerSessionSource()
  const session = await source.getSession('one')
  await expect(source.saveAnswer('one', session.id, answer(session.tasks[1].assignmentId))).rejects.toThrow()
  await source.saveAnswer('one', session.id, answer(session.tasks[0].assignmentId))
  await expect(source.saveAnswer('one', session.id, answer(session.tasks[0].assignmentId))).rejects.toThrow()
  expect((await source.getSession('one')).answers).toHaveLength(1)
  const restarted = await source.restart('one')
  expect(restarted.id).not.toBe(session.id)
  await expect(source.saveAnswer('one', session.id, answer(session.tasks[1].assignmentId))).rejects.toThrow()
  expect((await source.getSession('one')).answers).toHaveLength(0)
})

it('completes after seven answers, preserves results and protects snapshots from mutation', async () => {
  const source = createMockTrainerSessionSource()
  const session = await source.getSession('one')
  for (const task of session.tasks) await source.saveAnswer('one', session.id, answer(task.assignmentId, 1))
  const completed = await source.getSession('one')
  expect(completed.currentAssignmentId).toBeNull()
  expect(completed.answers.reduce((total, item) => total + item.assessment.score, 0)).toBe(7)
  completed.answers.length = 0
  expect((await source.getSession('one')).answers).toHaveLength(7)
  source.clear('one')
  expect((await source.getSession('one')).answers).toHaveLength(0)
})
