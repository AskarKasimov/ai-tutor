import type { QueryClient } from '@tanstack/react-query'
import type { User } from '../shared/domain'

export type SessionToken = { epoch: number; userId: string | null }
type SessionState = {
  epoch: number
  userId: string | null
  requests: Map<number, Set<AbortController>>
}
const states = new WeakMap<QueryClient, SessionState>()
function state(client: QueryClient): SessionState {
  let value = states.get(client)
  if (!value) {
    value = { epoch: 0, userId: null, requests: new Map() }
    states.set(client, value)
  }
  return value
}
export function captureSession(client: QueryClient): SessionToken {
  const value = state(client)
  return { epoch: value.epoch, userId: value.userId }
}
export function isCurrentSession(
  client: QueryClient,
  token: SessionToken,
): boolean {
  const value = state(client)
  return value.epoch === token.epoch && value.userId === token.userId
}
export function registerSessionRequest(
  client: QueryClient,
  token: SessionToken,
  controller: AbortController,
): () => void {
  const value = state(client)
  if (!isCurrentSession(client, token)) {
    controller.abort()
    return () => undefined
  }
  let requests = value.requests.get(token.epoch)
  if (!requests) {
    requests = new Set()
    value.requests.set(token.epoch, requests)
  }
  requests.add(controller)
  return () => {
    requests?.delete(controller)
    if (requests?.size === 0) value.requests.delete(token.epoch)
  }
}
export async function adoptCurrentUser(
  client: QueryClient,
  user: User | null,
  token: SessionToken,
): Promise<boolean> {
  if (!isCurrentSession(client, token)) return false
  const current = state(client)
  if (current.userId === user?.id) return true
  if (current.userId !== null || user !== null)
    return changeSession(client, user, token.epoch)
  return true
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
  const current = state(client)
  if (expectedEpoch !== undefined && current.epoch !== expectedEpoch)
    return false
  const oldEpoch = current.epoch
  current.epoch++
  current.userId = user?.id ?? null
  for (const controller of current.requests.get(oldEpoch) ?? [])
    controller.abort()
  current.requests.delete(oldEpoch)
  const keepAuth = (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] !== 'auth'
  client.getMutationCache().clear()
  const epoch = current.epoch
  await client.cancelQueries({ predicate: keepAuth })
  if (current.epoch !== epoch) return false
  client.removeQueries({ predicate: keepAuth })
  client.setQueryData(['auth', 'me'], user)
  return true
}
