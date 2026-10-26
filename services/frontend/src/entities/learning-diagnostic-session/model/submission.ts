import type { DiagnosticTask } from '@/entities/learning-diagnostic-session'

export type DiagnosticSubmission = {
  sessionId: string
  taskId: string
  role: DiagnosticTask['role']
  key: string
  body: FormData
  skip?: boolean
  transcriptionId?: string
}
