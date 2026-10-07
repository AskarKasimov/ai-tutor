import type { QueryClient } from '@tanstack/react-query'
import type { SessionToken } from './session-lifecycle'
import type { DiagnosticSessionIdentity } from '../platform/diagnostic-session-storage'
import type { DiagnosticProgress } from '../shared/domain'

type Entry = {
  controller: AbortController
  promise: Promise<DiagnosticProgress>
  subscribers: number
}
const registries = new WeakMap<QueryClient, Map<string, Entry>>()
export function acquireDiagnosticBootstrap(
  client: QueryClient,
  token: SessionToken,
  identity: DiagnosticSessionIdentity,
  run: (
    signal: AbortSignal,
    identity: DiagnosticSessionIdentity,
  ) => Promise<DiagnosticProgress>,
) {
  let registry = registries.get(client)
  if (!registry) {
    registry = new Map()
    registries.set(client, registry)
  }
  const key = `${token.userId ?? ''}:${token.epoch}`
  let entry = registry.get(key)
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
        if (registry?.get(key) === created) registry.delete(key)
      })
    entry = created
    registry.set(key, entry)
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
        if (acquired.subscribers === 0 && registry?.get(key) === acquired) {
          acquired.controller.abort()
          registry.delete(key)
        }
      })
    },
  }
}
