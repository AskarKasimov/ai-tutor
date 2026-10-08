import type { SessionToken } from '@/entities/user'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'

type Entry = {
  controller: AbortController
  promise: Promise<DiagnosticProgress>
  subscribers: number
}

export function createDiagnosticBootstrapCoordinator() {
  const entries = new Map<string, Entry>()
  return {
    acquire(
      token: SessionToken,
      identity: DiagnosticSessionIdentity,
      run: (
        signal: AbortSignal,
        identity: DiagnosticSessionIdentity,
      ) => Promise<DiagnosticProgress>,
    ) {
      const key = `${token.userId ?? ''}:${token.epoch}`
      let entry = entries.get(key)
      if (!entry) {
        const controller = new AbortController()
        const created: Entry = {
          controller,
          subscribers: 0,
          promise: Promise.resolve(undefined as never),
        }
        created.promise = Promise.resolve()
          .then(() => run(controller.signal, identity))
          .finally(() => {
            if (entries.get(key) === created) entries.delete(key)
          })
        entry = created
        entries.set(key, entry)
      }
      entry.subscribers++
      const acquired = entry
      let released = false
      return {
        promise: acquired.promise,
        release() {
          if (released) return
          released = true
          acquired.subscribers--
          queueMicrotask(() => {
            if (acquired.subscribers === 0 && entries.get(key) === acquired) {
              acquired.controller.abort()
              entries.delete(key)
            }
          })
        },
      }
    },
  }
}
