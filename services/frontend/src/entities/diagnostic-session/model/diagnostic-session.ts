import type { AssessmentVerdict } from '@/entities/assessment/@x/diagnostic-session'

export type DiagnosticTask = {
  variant_task_id: string
  source_task_id: string
  role: 'main' | 'basic'
  competency_id: string
  competency_name: string
  constituent_id: string
  constituent_name: string
  outcome_id: string
  outcome_name: string
  question: string
  options: string[]
  voice_instruction: string
}

export type DiagnosticProgress = {
  answer_skipped?: boolean
  text?: string
  session_id: string
  status: 'active' | 'completed'
  completed_tasks: number
  skipped_tasks: number
  total_tasks: number
  // Topics are fixed; questions vary with answers (main plus 0–2 basics).
  competency_count?: number
  current_competency?: number
  current_step?: number
  current?: DiagnosticTask
  score?: number
  grader_score?: number
  grader_max_score?: number
  verdict?: AssessmentVerdict
  criterion_results?: { key: string; satisfied: boolean; explanation: string }[]
  feedback?: string[]
}

export type DiagnosticAudioMetadata = {
  variant_task_id: string
  status:
    'missing' | 'pending' | 'processing' | 'ready' | 'failed' | 'cancelled'
  audio_url: string | null
}

export type DiagnosticAnswer = {
  skipped?: boolean
  variant_task_id: string
  source_task_id: string
  competency_id: string
  outcome_id: string
  role: 'main' | 'basic'
  task: DiagnosticTask
  transcription_id: string
  text: string
  grader_score: number
  grader_max_score: number
  score: number
  verdict: AssessmentVerdict
  criterion_results: { key: string; satisfied: boolean; explanation: string }[]
  feedback: string[]
  created_at: number
}

export type DiagnosticResult = {
  session_id: string
  status: 'completed'
  variant_id: string
  map_revision: number
  included_competency_count: number
  skipped_competencies: {
    competency_id: string
    competency_name: string
    code: string
  }[]
  completed_tasks: number
  total_tasks: number
  diagnostic_score: number
  maximum_score: number
  answers: DiagnosticAnswer[]
  untested_basics: DiagnosticTask[]
}

export type DiagnosticOverallFeedback = {
  session_id: string
  diagnostic_score: number
  maximum_score: number
  score_percentage: number
  summary: string
  strengths: string[]
  confirmed_gaps: {
    competency_id: string
    competency_name: string
    outcome_id: string
    outcome_name: string
    taxonomy_code: string
    importance: number
    failed_criteria: string[]
    advice: string
  }[]
  partial_competencies: {
    competency_id: string
    competency_name: string
    details: string
  }[]
  unverified_competencies: {
    competency_id: string
    competency_name: string
    code: string
  }[]
  training_recommendations: {
    competency_id: string
    competency_name: string
    outcome_id: string
    outcome_name: string
    priority: number
    rationale: string
  }[]
  generated_at: number
}
