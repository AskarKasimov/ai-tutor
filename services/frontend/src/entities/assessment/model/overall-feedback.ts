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
