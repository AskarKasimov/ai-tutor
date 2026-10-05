import { apiFetch } from './api-fetch'
import type { Assessment, AssessmentTask } from '../shared/domain'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')

export class AssessmentApiError extends Error {
  readonly status: number

  constructor(status: number) {
    super(`Assessment failed: ${status}`)
    this.name = 'AssessmentApiError'
    this.status = status
  }
}

export async function evaluateAnswer(transcriptionId: string, task: AssessmentTask, signal: AbortSignal): Promise<Assessment> {
  const response = await apiFetch(`${apiBase}/assessments/evaluate`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      transcription_id: transcriptionId,
      question: task.question,
      options: task.options,
      voice_instruction: task.voiceInstruction,
      correct_answer: task.correctAnswer,
    }),
    signal: AbortSignal.any([signal, AbortSignal.timeout(120_000)]),
  })
  if (!response.ok) throw new AssessmentApiError(response.status)
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object' || !('score' in data) || typeof data.score !== 'number' || !Number.isInteger(data.score) || data.score < 0 || data.score > 2 || !('feedback' in data) || !Array.isArray(data.feedback) || data.feedback.length !== 3 || !data.feedback.every((line) => typeof line === 'string' && line.trim())) {
    throw new Error('Invalid assessment response')
  }
  return { score: data.score as 0 | 1 | 2, feedback: data.feedback as [string, string, string] }
}
