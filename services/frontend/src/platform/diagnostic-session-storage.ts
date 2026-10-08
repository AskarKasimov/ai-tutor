import { z } from 'zod'

const schema = z.object({
  variantKey: z.string().min(1),
  startKey: z.string().min(1),
  variantId: z.string().optional(),
  sessionId: z.string().optional(),
})
export type DiagnosticSessionIdentity = z.infer<typeof schema>
const memory = new Map<string, DiagnosticSessionIdentity>()
function storageKey(userId: string, apiBase: string) {
  return `ai-tutor:diagnostic:${apiBase}:${userId}`
}
export function loadDiagnosticIdentity(
  userId: string,
  apiBase: string,
): DiagnosticSessionIdentity | undefined {
  const key = storageKey(userId, apiBase)
  try {
    const raw = sessionStorage.getItem(key)
    if (raw) {
      const parsed = schema.safeParse(JSON.parse(raw))
      if (parsed.success) return parsed.data
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
  identity: DiagnosticSessionIdentity,
) {
  const key = storageKey(userId, apiBase)
  memory.set(key, identity)
  try {
    sessionStorage.setItem(key, JSON.stringify(identity))
  } catch {
    /* Memory fallback. */
  }
}
