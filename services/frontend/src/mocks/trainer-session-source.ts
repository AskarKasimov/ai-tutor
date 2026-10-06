import type { TrainerSessionSource } from '../data/trainer-session-source'
import type { SessionTask, TrainerSession } from '../shared/domain'

const modelOptions = ['classification', 'regression', 'clustering', 'ranking'].map((name) => `trainer.options.${name}`)
const tasks: Omit<SessionTask, 'assignmentId'>[] = [
  { taskId: 'ml_001', questionKey: 'trainer.question', optionKeys: modelOptions, instructionKey: 'trainer.voiceInstruction' },
  { taskId: 'ml_002', questionKey: 'session.tasks.price.question', optionKeys: [modelOptions[1], modelOptions[0], ...modelOptions.slice(2)], instructionKey: 'session.voiceInstruction' },
  { taskId: 'ml_014', questionKey: 'session.tasks.segments.question', optionKeys: modelOptions, instructionKey: 'session.voiceInstruction' },
  { taskId: 'ml_015', questionKey: 'session.tasks.search.question', optionKeys: modelOptions, instructionKey: 'session.voiceInstruction' },
  { taskId: 'ml_016', questionKey: 'session.tasks.overfitting.question', optionKeys: ['a', 'b', 'c', 'd'].map((letter) => `session.tasks.overfitting.${letter}`), instructionKey: 'session.voiceInstruction' },
  { taskId: 'ml_017', questionKey: 'session.tasks.split.question', optionKeys: ['a', 'b', 'c', 'd'].map((letter) => `session.tasks.split.${letter}`), instructionKey: 'session.voiceInstruction' },
  { taskId: 'ml_018', questionKey: 'session.tasks.metrics.question', optionKeys: ['a', 'b', 'c', 'd'].map((letter) => `session.tasks.metrics.${letter}`), instructionKey: 'session.voiceInstruction' },
]

export function createMockTrainerSessionSource(): TrainerSessionSource {
  const sessions = new Map<string, TrainerSession>()
  function create(userId: string) {
    const id = crypto.randomUUID()
    const assignments = tasks.map((task, index) => ({ ...task, assignmentId: `${id}-${index + 1}` }))
    const session: TrainerSession = { id, userId, tasks: assignments, answers: [], currentAssignmentId: assignments[0].assignmentId }
    sessions.set(userId, session)
    return session
  }
  return {
    async getSession(userId) { return structuredClone(sessions.get(userId) ?? create(userId)) },
    async saveAnswer(userId, sessionId, answer) {
      const session = sessions.get(userId)
      if (!session || session.id !== sessionId || session.currentAssignmentId !== answer.assignmentId) throw new Error('Inactive assignment')
      session.answers.push(structuredClone(answer))
      session.currentAssignmentId = session.tasks[session.answers.length]?.assignmentId ?? null
      return structuredClone(session)
    },
    async restart(userId) { return structuredClone(create(userId)) },
    clear(userId) { sessions.delete(userId) },
  }
}
