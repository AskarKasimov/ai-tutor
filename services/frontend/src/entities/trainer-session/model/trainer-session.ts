import type { Assessment } from '@/entities/assessment/@x/trainer-session'

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
