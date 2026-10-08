export type AssessmentTask = {
  variantId: string
  variantTaskId: string
  question: string
  options: string[]
  voiceInstruction: string
}

export type DemoAssessmentTask = Omit<
  AssessmentTask,
  'variantId' | 'variantTaskId'
> & {
  taskId: string
}

export type AssessmentVerdict = 'correct' | 'partial' | 'incorrect'

export type CriterionResult = {
  key: string
  satisfied: boolean
  explanation: string
}

export type Assessment = {
  score: 0 | 1 | 2
  maxScore?: 1 | 2
  feedback: [string, string, string]
  verdict?: AssessmentVerdict
  criterionResults?: CriterionResult[]
}
