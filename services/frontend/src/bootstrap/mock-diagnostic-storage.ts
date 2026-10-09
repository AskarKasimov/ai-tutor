import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'

const identities = new Map<string, DiagnosticSessionIdentity>()
const pendingIdentities = new Map<string, DiagnosticSessionIdentity>()

function key(userId: string, apiBase: string, subjectId: string) {
  return `${apiBase}:${userId}:${subjectId}`
}

export const mockDiagnosticStorage = {
  load(userId: string, apiBase: string, subjectId: string) {
    const value = identities.get(key(userId, apiBase, subjectId))
    return value ? { ...value } : undefined
  },
  save(
    userId: string,
    apiBase: string,
    subjectId: string,
    identity: DiagnosticSessionIdentity,
  ) {
    if (identity.subjectId === subjectId)
      identities.set(key(userId, apiBase, subjectId), { ...identity })
  },
  loadPending(userId: string, apiBase: string, subjectId: string) {
    const value = pendingIdentities.get(key(userId, apiBase, subjectId))
    return value ? { ...value } : undefined
  },
  savePending(
    userId: string,
    apiBase: string,
    subjectId: string,
    identity: DiagnosticSessionIdentity,
  ) {
    if (identity.subjectId === subjectId)
      pendingIdentities.set(key(userId, apiBase, subjectId), { ...identity })
  },
  clearPending(userId: string, apiBase: string, subjectId: string) {
    pendingIdentities.delete(key(userId, apiBase, subjectId))
  },
  clear() {
    identities.clear()
    pendingIdentities.clear()
  },
}
