export type AssessmentTask = {
  question: string
  options: string[]
  voiceInstruction: string
  correctAnswer?: string
}

export type Assessment = {
  score: 0 | 1 | 2
  feedback: [string, string, string]
}
