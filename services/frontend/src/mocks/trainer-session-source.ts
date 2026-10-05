import type { TrainerSessionSource } from '../data/trainer-session-source'
import type { SessionTask, TrainerSession } from '../shared/domain'

const modelOptions = ['classification', 'regression', 'clustering', 'ranking'].map((name) => `trainer.options.${name}`)
const tasks: Omit<SessionTask, 'assignmentId'>[] = [
  { taskId: 'credit', questionKey: 'trainer.question', optionKeys: modelOptions, instructionKey: 'trainer.voiceInstruction' },
  { taskId: 'price', questionKey: 'session.tasks.price.question', optionKeys: [modelOptions[1], modelOptions[0], ...modelOptions.slice(2)], instructionKey: 'session.voiceInstruction' },
  { taskId: 'segments', questionKey: 'session.tasks.segments.question', optionKeys: modelOptions, instructionKey: 'session.voiceInstruction' },
  { taskId: 'search', questionKey: 'session.tasks.search.question', optionKeys: modelOptions, instructionKey: 'session.voiceInstruction' },
  ...(['overfitting', 'split', 'metrics'] as const).map((name) => ({
    taskId: name, questionKey: `session.tasks.${name}.question`,
    optionKeys: ['a', 'b', 'c', 'd'].map((letter) => `session.tasks.${name}.${letter}`), instructionKey: 'session.voiceInstruction',
  })),
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
