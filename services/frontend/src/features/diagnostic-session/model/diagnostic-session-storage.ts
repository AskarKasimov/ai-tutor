import { z } from 'zod'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'

const schema = z.object({
  subjectId: z.string().min(1),
  variantKey: z.string().min(1),
  startKey: z.string().min(1),
  variantId: z.string().optional(),
  sessionId: z.string().optional(),
})
const memory = new Map<string, DiagnosticSessionIdentity>()
function storageKey(userId: string, apiBase: string, subjectId: string) {
  return `ai-tutor:diagnostic:${apiBase}:${userId}:${subjectId}`
}
export function loadDiagnosticIdentity(
  userId: string,
  apiBase: string,
  subjectId: string,
): DiagnosticSessionIdentity | undefined {
  const key = storageKey(userId, apiBase, subjectId)
  try {
    const raw = sessionStorage.getItem(key)
    if (raw) {
      const parsed = schema.safeParse(JSON.parse(raw))
      if (parsed.success && parsed.data.subjectId === subjectId)
        return parsed.data
    }
    memory.delete(key)
    return undefined
  } catch {
    /* Storage may be unavailable; keep this tab's session in memory. */
  }
  return memory.get(key)
}
export function saveDiagnosticIdentity(
  userId: string,
  apiBase: string,
  subjectId: string,
  identity: DiagnosticSessionIdentity,
) {
  if (identity.subjectId !== subjectId) return
  const key = storageKey(userId, apiBase, subjectId)
  memory.set(key, identity)
  try {
    sessionStorage.setItem(key, JSON.stringify(identity))
  } catch {
    /* Memory fallback. */
  }
}
