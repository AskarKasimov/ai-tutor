import { MutationObserver, QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from './session-lifecycle'

describe('session lifecycle', () => {
  it('invalidates an earlier request even when the same user logs in again', async () => {
    const client = new QueryClient()
    await replaceSession(client, {
      id: 'same',
      email: 'a@example.com',
      display_name: null,
      role: 'student',
    })
    const oldSession = captureSession(client)
    const controller = new AbortController()
    registerSessionRequest(client, oldSession, controller)
    await replaceSession(client, {
      id: 'same',
      email: 'a@example.com',
      display_name: null,
      role: 'student',
    })
    expect(controller.signal.aborted).toBe(true)
    expect(isCurrentSession(client, oldSession)).toBe(false)
  })
})

it('removes a completed observed auth mutation when the session changes', async () => {
  const client = new QueryClient()
  const observer = new MutationObserver(client, {
    mutationKey: ['auth', 'authenticate'],
    gcTime: 0,
    mutationFn: async (input: { password: string }) => ({
      id: input.password === 'secret' ? 'same' : 'other',
      email: 'a@example.com',
      display_name: null,
      role: 'student' as const,
    }),
  })
  const unsubscribe = observer.subscribe(() => undefined)
  await observer.mutate({ password: 'secret' })
  expect(client.getMutationCache().getAll()).toHaveLength(1)
  await replaceSession(client, null)
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  unsubscribe()
})
