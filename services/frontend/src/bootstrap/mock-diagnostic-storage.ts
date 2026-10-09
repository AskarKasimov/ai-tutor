import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'

const identities = new Map<string, DiagnosticSessionIdentity>()

function key(userId: string, apiBase: string) {
  return `${apiBase}:${userId}`
}

export const mockDiagnosticStorage = {
  load(userId: string, apiBase: string) {
    const value = identities.get(key(userId, apiBase))
    return value ? { ...value } : undefined
  },
  save(userId: string, apiBase: string, identity: DiagnosticSessionIdentity) {
    identities.set(key(userId, apiBase), { ...identity })
  },
  clear() {
    identities.clear()
  },
}
