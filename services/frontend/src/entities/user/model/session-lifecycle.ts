import type { QueryClient } from '@tanstack/react-query'
import { isAdminPortal } from '@/shared/api'
import { userQueryKeys } from './query-keys'
import type { User } from './user'
import { SessionRegistry } from './session-registry'
import type { Portal, SessionToken } from './session-registry'
export type { SessionToken } from './session-registry'
const registries = new WeakMap<QueryClient, Map<Portal, SessionRegistry>>()
function portal(): Portal {
  return isAdminPortal() ? 'admin' : 'student'
}
function registry(client: QueryClient, forPortal = portal()) {
  let byPortal = registries.get(client)
  if (!byPortal) {
    byPortal = new Map()
    registries.set(client, byPortal)
  }
  let value = byPortal.get(forPortal)
  if (!value) {
    value = new SessionRegistry(forPortal)
    byPortal.set(forPortal, value)
  }
  return value
}
export function captureSession(client: QueryClient): SessionToken {
  return registry(client).capture()
}
export function isCurrentSession(client: QueryClient, token: SessionToken) {
  return registry(client, token.portal).isCurrent(token)
}
export function registerSessionRequest(
  client: QueryClient,
  token: SessionToken,
  controller: AbortController,
): () => void {
  return registry(client, token.portal).register(token, controller)
}
export async function adoptCurrentUser(
  client: QueryClient,
  user: User | null,
  token: SessionToken,
): Promise<boolean> {
  const sessions = registry(client, token.portal)
  if (!sessions.isCurrent(token)) return false
  if (token.userId === (user?.id ?? null)) return true
  return changeSession(client, user, token.epoch, token.portal)
}
export async function replaceSession(
  client: QueryClient,
  user: User | null,
  forPortal: Portal = portal(),
): Promise<void> {
  await changeSession(client, user, undefined, forPortal)
}
async function changeSession(
  client: QueryClient,
  user: User | null,
  expectedEpoch: number | undefined,
  forPortal: Portal,
): Promise<boolean> {
  const sessions = registry(client, forPortal)
  if (!sessions.replace(user?.id ?? null, expectedEpoch)) return false
  const epoch = sessions.capture().epoch
  const keepAuth = (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] === 'auth'
  for (const mutation of client.getMutationCache().getAll()) {
    const key = mutation.options.mutationKey
    if (
      key?.[0] === 'auth' &&
      (key[2] === forPortal || (key.length === 2 && forPortal === 'student'))
    )
      client.getMutationCache().remove(mutation)
  }
  // Auth mutations and cache entries for the other portal retain their own cookie session.
  await client.cancelQueries({
    predicate: (query) => !keepAuth(query as { queryKey: readonly unknown[] }),
  })
  if (sessions.capture().epoch !== epoch) return false
  client.removeQueries({
    predicate: (query) => !keepAuth(query as { queryKey: readonly unknown[] }),
  })
  client.setQueryData(userQueryKeys.authForPortal(forPortal === 'admin'), user)
  return true
}
