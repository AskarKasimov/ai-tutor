
export type AssessmentTask = {
  taskId: string
  question: string
  options: string[]
  voiceInstruction: string
}

export type AssessmentVerdict = 'correct' | 'partial' | 'incorrect'

export type CriterionResult = {
  key: string
  satisfied: boolean
  explanation: string
}

export type Assessment = {
  score: 0 | 1 | 2
  feedback: [string, string, string]
  verdict?: AssessmentVerdict
  criterionResults?: CriterionResult[]
}

export type SessionTask = {
  assignmentId: string
  taskId: string
  questionKey: string
  optionKeys: string[]
  instructionKey: string
}

export type SessionAnswer = {
  assignmentId: string
  transcript: string
  assessment: Assessment
  submittedAt: number
}

export type TrainerSession = {
  id: string
  userId: string
  tasks: SessionTask[]
  answers: SessionAnswer[]
  currentAssignmentId: string | null
}
