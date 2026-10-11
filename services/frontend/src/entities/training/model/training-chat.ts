export type TrainingChatRole = 'user' | 'assistant'
export type TrainingChatKind = 'welcome' | 'message' | 'check'
export type TrainingChatSource = 'text' | 'voice'
export type TrainingChatStatus = 'active' | 'closed'
export type TrainingChatTurnStatus =
  'processing' | 'failed' | 'completed' | 'cancelled'

export type TrainingChatMessage = {
  id: string
  sequence: number
  role: TrainingChatRole
  kind: TrainingChatKind
  text: string
  source?: TrainingChatSource
  created_at: number
}

export type TrainingChatCheck = {
  revision: number
  final_answer: string
  source: TrainingChatSource
  transcription_id?: string | null
  score: 0 | 1 | 2
  max_score: 2
  verdict: 'incorrect' | 'partial' | 'correct'
}

export type TrainingChatPendingTurn = {
  id: string
  kind: 'message' | 'check'
  source: TrainingChatSource
  status: 'processing' | 'failed'
  user_text: string | null
  retry_after_ms: number
}

export type TrainingChatPage = {
  session_id: string
  exercise_id: string
  revision: number
  status: TrainingChatStatus
  items: TrainingChatMessage[]
  next_cursor?: string | null
  latest_check: TrainingChatCheck | null
  pending_turn: TrainingChatPendingTurn | null
}

export type TrainingChatTurnResult = {
  session_id: string
  exercise_id: string
  revision: number
  turn_id: string
  messages: [TrainingChatMessage, TrainingChatMessage]
  latest_check: TrainingChatCheck | null
}
