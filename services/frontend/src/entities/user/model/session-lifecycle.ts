import type { QueryClient } from '@tanstack/react-query'
import { userQueryKeys } from './query-keys'
import type { User } from './user'
import { SessionRegistry } from './session-registry'
import type { SessionToken } from './session-registry'

export type { SessionToken } from './session-registry'
const registries = new WeakMap<QueryClient, SessionRegistry>()
function registry(client: QueryClient) {
  let value = registries.get(client)
  if (!value) {
    value = new SessionRegistry()
    registries.set(client, value)
  }
  return value
}
export function captureSession(client: QueryClient): SessionToken {
  return registry(client).capture()
}
export function isCurrentSession(client: QueryClient, token: SessionToken) {
  return registry(client).isCurrent(token)
}
export function registerSessionRequest(
  client: QueryClient,
  token: SessionToken,
  controller: AbortController,
): () => void {
  return registry(client).register(token, controller)
}
export async function adoptCurrentUser(
  client: QueryClient,
  user: User | null,
  token: SessionToken,
): Promise<boolean> {
  const sessions = registry(client)
  if (!sessions.isCurrent(token)) return false
  if (token.userId === (user?.id ?? null)) return true
  return changeSession(client, user, token.epoch)
}
export async function replaceSession(
  client: QueryClient,
  user: User | null,
): Promise<void> {
  await changeSession(client, user)
}
async function changeSession(
  client: QueryClient,
  user: User | null,
  expectedEpoch?: number,
): Promise<boolean> {
  const sessions = registry(client)
  if (!sessions.replace(user?.id ?? null, expectedEpoch)) return false
  const keepAuth = (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] !== userQueryKeys.auth[0]
  client.getMutationCache().clear()
  const epoch = sessions.capture().epoch
  await client.cancelQueries({ predicate: keepAuth })
  if (sessions.capture().epoch !== epoch) return false
  client.removeQueries({ predicate: keepAuth })
  client.setQueryData(userQueryKeys.auth, user)
  return true
}
