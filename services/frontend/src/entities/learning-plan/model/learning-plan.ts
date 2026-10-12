// A personal learning plan: every outcome of the competencies the diagnostic
// found weak, the important ones first while the student's hours last.
export type LearningItemStatus = 'pending' | 'current' | 'passed' | 'review'

export type LearningItem = {
  index: number
  competency_id: string
  competency_name: string
  outcome_id: string
  outcome_name: string
  theory_minutes: number
  practice_minutes: number
  // Outcomes beyond the hours are studied "if time remains".
  in_plan: boolean
  status: LearningItemStatus
  attempts: number
}

export type LearningPlan = {
  id: string
  diagnostic_session_id: string
  subject_id: string
  subject_name: string
  hours: number
  detail: 'brief' | 'standard' | 'deep'
  status: 'active' | 'completed'
  current_index: number | null
  progress_percent: number
  planned_minutes: number
  // A transcribed answer whose check failed and waits for a retry.
  pending_answer: string | null
  items: LearningItem[]
}

export type LearningMessage = {
  sequence: number
  item_index: number
  role: 'assistant' | 'user'
  kind: 'theory' | 'practice' | 'answer' | 'feedback'
  text: string
  created_at: number
}
