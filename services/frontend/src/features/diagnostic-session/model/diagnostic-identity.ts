import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'

export function createDiagnosticIdentity(): DiagnosticSessionIdentity {
  return { variantKey: crypto.randomUUID(), startKey: crypto.randomUUID() }
}
