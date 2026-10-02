import type { GeneratedTasks, TrainingOutcome } from '../shared/domain'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')

export class TaskGenerationApiError extends Error {
  readonly status: number

  constructor(status: number) {
    super(`Task generation failed: ${status}`)
    this.name = 'TaskGenerationApiError'
    this.status = status
  }
}

function requestSignal(signal?: AbortSignal) {
  const timeout = AbortSignal.timeout(120_000)
  return signal ? AbortSignal.any([signal, timeout]) : timeout
}

type RawOutcome = { id: string; name: string; constituent_name: string; competency_name: string; task_count: number }
type RawTask = { question: string; criteria: string; voice_instruction: string; options: string[] }
type RawGenerated = { outcome_id: string; outcome_name: string; tasks: RawTask[] }

function isOutcome(value: unknown): value is RawOutcome {
  if (!value || typeof value !== 'object') return false
  const item = value as Record<string, unknown>
  return typeof item.id === 'string' && item.id.trim() !== ''
    && typeof item.name === 'string' && item.name.trim() !== ''
    && typeof item.constituent_name === 'string' && typeof item.competency_name === 'string'
    && typeof item.task_count === 'number' && Number.isInteger(item.task_count) && item.task_count >= 0
}

function isTask(value: unknown): value is RawTask {
  if (!value || typeof value !== 'object') return false
  const item = value as Record<string, unknown>
  return typeof item.question === 'string' && item.question.trim() !== ''
    && typeof item.criteria === 'string' && item.criteria.trim() !== ''
    && typeof item.voice_instruction === 'string' && item.voice_instruction.trim() !== ''
    && Array.isArray(item.options) && item.options.every((option) => typeof option === 'string' && option.trim() !== '')
}

export async function listOutcomes(signal?: AbortSignal): Promise<TrainingOutcome[]> {
  const response = await fetch(`${apiBase}/outcomes`, { credentials: 'include', signal: requestSignal(signal) })
  if (!response.ok) throw new TaskGenerationApiError(response.status)
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object' || !('outcomes' in data) || !Array.isArray(data.outcomes) || !data.outcomes.every(isOutcome)) {
    throw new Error('Invalid outcomes response')
  }
  return data.outcomes.map((outcome) => ({
    id: outcome.id,
    name: outcome.name,
    constituentName: outcome.constituent_name,
    competencyName: outcome.competency_name,
    taskCount: outcome.task_count,
  }))
}

export async function generateTrainingTasks(outcomeId: string, count: number, signal?: AbortSignal): Promise<GeneratedTasks> {
  const response = await fetch(`${apiBase}/outcomes/${encodeURIComponent(outcomeId)}/training-tasks`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ count }),
    signal: requestSignal(signal),
  })
  if (!response.ok) throw new TaskGenerationApiError(response.status)
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object' || !('outcome_id' in data) || typeof data.outcome_id !== 'string'
    || !('outcome_name' in data) || typeof data.outcome_name !== 'string'
    || !('tasks' in data) || !Array.isArray(data.tasks) || !data.tasks.every(isTask)) {
    throw new Error('Invalid training tasks response')
  }
  const generated = data as RawGenerated
  return {
    outcomeId: generated.outcome_id,
    outcomeName: generated.outcome_name,
    tasks: generated.tasks.map((task) => ({
      question: task.question,
      criteria: task.criteria,
      voiceInstruction: task.voice_instruction,
      options: task.options,
    })),
  }
}