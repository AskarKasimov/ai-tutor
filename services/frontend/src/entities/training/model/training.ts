export type TrainingTarget = {
  kind: string
  label: string
  competency_id: string
  competency_name: string
  outcome_id: string
  outcome_name: string
  original_score: number | null
  original_max_score: number | null
  last_score: number | null
  /** The main task of the block was solved during training. */
  mastered?: boolean
}

/**
 * Block stages: the main task first, theory after a failed main task, then
 * basic tasks. `practice` marks sessions created before blocks existed.
 */
export type TrainingStage = 'main' | 'theory' | 'basic' | 'practice'

export type TrainingExercise = {
  exercise_id: string
  outcome_id: string
  outcome_name: string
  question: string
  options: string[]
  voice_instruction: string
  stage?: TrainingStage
  competency_name?: string
  /** Theory summary of every ОС of the competency; only on the theory stage. */
  theory?: string
  basic_index?: number
  basic_count?: number
}

export type TrainingCriterionResult = {
  key: string
  satisfied: boolean
  explanation: string
}
export type TrainingAttempt = {
  skipped?: boolean
  sequence: number
  exercise_id: string
  round: number
  target_index: number
  transcription_id?: string
  answer_source?: 'text' | 'voice'
  text: string
  score: number
  max_score: number
  verdict: 'correct' | 'partial' | 'incorrect'
  criterion_results: TrainingCriterionResult[]
  feedback: string[]
  created_at: number
}

export type TrainingProgress = {
  session_id: string
  diagnostic_session_id: string
  subject_id: string
  subject_name: string
  mode: 'focused' | 'free_practice'
  status: 'active'
  round: number
  answer_count: number
  targets: TrainingTarget[]
  current: TrainingExercise
  answer?: TrainingAttempt
}

export type TrainingPreview = {
  diagnostic_session_id: string
  subject_id: string
  subject_name: string
  mode: 'focused' | 'free_practice'
  plan_revision: number
  diagnostic_score: number
  maximum_score: number
  status: 'ready' | 'no_practice_tasks'
  confirmed_gaps: TrainingTarget[]
  partial_competencies: TrainingTarget[]
  topics: TrainingTarget[]
}

export type TrainingHistory = {
  items: TrainingAttempt[]
  targets: TrainingTarget[]
  next_cursor?: string
}

export type TrainingAudioMetadata = {
  exercise_id: string
  status:
    'missing' | 'pending' | 'processing' | 'ready' | 'failed' | 'cancelled'
  audio_url: string | null
}

export type TrainingSubmission = {
  sessionId: string
  exerciseId: string
  key: string
  body: FormData
  skip?: boolean
}
