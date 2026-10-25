export type Subject = { id: string; name: string; ready: boolean }

export type LearningState = {
  subject_id: string
  subject_name: string
  diagnostic_status: 'not_started' | 'active' | 'completed'
  diagnostic_completed: boolean
  training_available: boolean
  diagnostic_session_id?: string
  active_session_id?: string
}
