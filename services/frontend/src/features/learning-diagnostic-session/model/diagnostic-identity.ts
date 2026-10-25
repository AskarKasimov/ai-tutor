import type { DiagnosticSessionIdentity } from '@/entities/learning-diagnostic-session'

export function createDiagnosticIdentity(
  subjectId: string,
): DiagnosticSessionIdentity {
  return {
    subjectId,
    variantKey: crypto.randomUUID(),
    startKey: crypto.randomUUID(),
  }
}
