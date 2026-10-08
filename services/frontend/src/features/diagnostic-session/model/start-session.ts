import type { DiagnosticProgress } from '@/entities/diagnostic-session'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'
import type { SessionToken } from '@/entities/user'

export type StartDiagnosticSessionDependencies = {
  apiBase: string
  api: {
    createVariant(key: string, signal: AbortSignal): Promise<{ id: string }>
    startDiagnostic(
      variantId: string,
      key: string,
      signal: AbortSignal,
    ): Promise<DiagnosticProgress>
  }
  sessions: {
    capture(): SessionToken
    isCurrent(token: SessionToken): boolean
    register(token: SessionToken, controller: AbortController): () => void
  }
  storage: {
    load(userId: string, apiBase: string): DiagnosticSessionIdentity | undefined
    save(
      userId: string,
      apiBase: string,
      identity: DiagnosticSessionIdentity,
    ): void
  }
}

export async function startDiagnosticSession(
  userId: string,
  identity: DiagnosticSessionIdentity,
  signal: AbortSignal,
  dependencies: StartDiagnosticSessionDependencies,
) {
  const token = dependencies.sessions.capture()
  const controller = new AbortController()
  const unregister = dependencies.sessions.register(token, controller)
  const combined = AbortSignal.any([signal, controller.signal])
  function checkSession() {
    combined.throwIfAborted()
    if (!dependencies.sessions.isCurrent(token))
      throw new DOMException('Session changed', 'AbortError')
  }
  try {
    checkSession()
    const saved = dependencies.storage.load(userId, dependencies.apiBase)
    let current = saved?.startKey === identity.startKey ? saved : identity
    if (!current.variantId) {
      const variant = await dependencies.api.createVariant(
        current.variantKey,
        combined,
      )
      checkSession()
      current = { ...current, variantId: variant.id }
      dependencies.storage.save(userId, dependencies.apiBase, current)
    }
    const progress = await dependencies.api.startDiagnostic(
      current.variantId!,
      current.startKey,
      combined,
    )
    checkSession()
    current = { ...current, sessionId: progress.session_id }
    dependencies.storage.save(userId, dependencies.apiBase, current)
    return { progress, identity: current, token }
  } finally {
    unregister()
  }
}
