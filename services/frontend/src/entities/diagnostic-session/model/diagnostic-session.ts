// The Geek oral exam as the API returns it, in camelCase.

export type Speaker = 'geek' | 'student'
export type TurnKind =
  'question' | 'followup' | 'rephrase' | 'closing' | 'answer'
export type AnswerSource = 'voice' | 'text' | 'dont_know'

// One line of the transcript.
export type Turn = {
  id: string
  line: number
  speaker: Speaker
  kind: TurnKind
  nodeId: string | null
  source: AnswerSource | null
  text: string
  createdAt: number
}

export type SessionStatus = 'active' | 'completed' | 'cancelled'

export type DiagnosticSession = {
  id: string
  subjectId: string
  status: SessionStatus
  progressPercent: number
  // The Geek turn awaiting an answer; null unless the session is active.
  currentTurnId: string | null
  startedAt: number
  completedAt: number | null
  turns: Turn[]
}

export type Grade = 'A' | 'B' | 'C' | 'D'
export type TopicStatus =
  | 'independent'
  | 'assisted'
  | 'gap'
  | 'insufficient'
  | 'credited_by_parent'
  | 'unchecked'

export type TopicResult = {
  nodeId: string
  title: string
  status: TopicStatus
  grade: Grade | null
  topLevel: boolean
}

export type DiagnosticResult = {
  sessionId: string
  grade: Grade | null
  scorePercent: number | null
  topics: TopicResult[]
  gaps: { nodeId: string; title: string }[]
}

// A past or current diagnostic of the student on the course.
export type DiagnosticSummary = {
  id: string
  status: SessionStatus
  progressPercent: number
  startedAt: number
  completedAt: number | null
  grade: Grade | null
  scorePercent: number | null
}

// The reply to the current Geek turn.
export type DiagnosticAnswer =
  | { kind: 'text'; text: string }
  | { kind: 'voice'; transcriptionId: string }
  | { kind: 'dont_know' }
