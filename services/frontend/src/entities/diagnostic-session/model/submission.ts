import type { DiagnosticTask } from '@/entities/diagnostic-session'

export type DiagnosticSubmission = {
  sessionId: string
  taskId: string
  role: DiagnosticTask['role']
  key: string
  body: FormData
}
