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

export type TrainingOutcome = {
  id: string
  name: string
  constituentName: string
  competencyName: string
  taskCount: number
}

export type GeneratedTask = {
  question: string
  criteria: string
  voiceInstruction: string
  options: string[]
}

export type GeneratedTasks = {
  outcomeId: string
  outcomeName: string
  tasks: GeneratedTask[]
}
