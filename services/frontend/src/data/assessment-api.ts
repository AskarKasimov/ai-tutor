import { apiFetch } from './api-fetch'
import type {
  Assessment,
  AssessmentTask,
  AssessmentVerdict,
  CriterionResult,
} from '../shared/domain'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

export class AssessmentApiError extends Error {
  readonly status: number

  constructor(status: number) {
    super(`Assessment failed: ${status}`)
    this.name = 'AssessmentApiError'
    this.status = status
  }
}

function isVerdict(value: unknown): value is AssessmentVerdict {
  return value === 'correct' || value === 'partial' || value === 'incorrect'
}

function isCriterionResult(value: unknown): value is CriterionResult {
  return (
    !!value &&
    typeof value === 'object' &&
    'key' in value &&
    typeof value.key === 'string' &&
    !!value.key.trim() &&
    'satisfied' in value &&
    typeof value.satisfied === 'boolean' &&
    'explanation' in value &&
    typeof value.explanation === 'string' &&
    !!value.explanation.trim()
  )
}

export async function evaluateAnswer(
  transcriptionId: string,
  task: AssessmentTask,
  signal: AbortSignal,
): Promise<Assessment> {
  const response = await apiFetch(`${apiBase}/assessments/evaluate`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      transcription_id: transcriptionId,
      task_id: task.taskId,
    }),
    signal: AbortSignal.any([signal, AbortSignal.timeout(120_000)]),
  })
  if (!response.ok) throw new AssessmentApiError(response.status)

  const data: unknown = await response.json()
  if (
    !data ||
    typeof data !== 'object' ||
    !('score' in data) ||
    typeof data.score !== 'number' ||
    !Number.isInteger(data.score) ||
    data.score < 0 ||
    data.score > 2 ||
    !('verdict' in data) ||
    !isVerdict(data.verdict) ||
    !('criterion_results' in data) ||
    !Array.isArray(data.criterion_results) ||
    data.criterion_results.length === 0 ||
    !data.criterion_results.every(isCriterionResult) ||
    !('feedback' in data) ||
    !Array.isArray(data.feedback) ||
    data.feedback.length !== 3 ||
    !data.feedback.every((line) => typeof line === 'string' && line.trim())
  ) {
    throw new Error('Invalid assessment response')
  }

  return {
    score: data.score as 0 | 1 | 2,
    verdict: data.verdict,
    criterionResults: data.criterion_results,
    feedback: data.feedback as [string, string, string],
  }
}

export interface OverallFeedbackItemInput {
  task_id: string
  topic?: string
  question: string
  transcript: string
  score: number
  max_score: number
  feedback: string[]
}

export interface OverallFeedbackData {
  score: number
  max_score: number
  score_percentage: number
  summary: string
  strengths: string[]
  gaps: string[]
  partials: string[]
  recommendations: string[]
  generated_at?: number
}

export async function fetchOverallFeedback(
  answers: OverallFeedbackItemInput[],
  signal?: AbortSignal
): Promise<OverallFeedbackData> {
  const response = await apiFetch(`${apiBase}/assessments/overall-feedback`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ answers }),
    signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(60_000)]) : AbortSignal.timeout(60_000),
  })
  if (!response.ok) throw new AssessmentApiError(response.status)

  const data = await response.json() as OverallFeedbackData
  return {
    score: typeof data.score === 'number' ? data.score : 0,
    max_score: typeof data.max_score === 'number' ? data.max_score : 14,
    score_percentage: typeof data.score_percentage === 'number' ? data.score_percentage : 0,
    summary: typeof data.summary === 'string' ? data.summary : '',
    strengths: Array.isArray(data.strengths) ? data.strengths : [],
    gaps: Array.isArray(data.gaps) ? data.gaps : [],
    partials: Array.isArray(data.partials) ? data.partials : [],
    recommendations: Array.isArray(data.recommendations) ? data.recommendations : [],
    generated_at: data.generated_at,
  }
}
