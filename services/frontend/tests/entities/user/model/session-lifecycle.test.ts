import { MutationObserver, QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { userQueryKeys } from '@/entities/user'
import {
  adoptCurrentUser,
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'

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

it('keeps the anonymous session epoch when the current user is still null', async () => {
  const client = new QueryClient()
  const token = captureSession(client)
  await expect(adoptCurrentUser(client, null, token)).resolves.toBe(true)
  expect(captureSession(client)).toEqual(token)
})

it('keeps student and admin authentication sessions separate in one query client', async () => {
  const client = new QueryClient()
  window.history.replaceState({}, '', '/')
  const student = {
    id: 'shared',
    email: 'a@example.com',
    display_name: null,
    role: 'student' as const,
  }
  await replaceSession(client, student)
  const studentToken = captureSession(client)
  window.history.replaceState({}, '', '/admin/courses')
  const admin = {
    id: 'shared',
    email: 'a@example.com',
    display_name: null,
    role: 'admin' as const,
  }
  await replaceSession(client, admin)
  const adminToken = captureSession(client)
  expect(studentToken.portal).toBe('student')
  expect(adminToken.portal).toBe('admin')
  expect(client.getQueryData(userQueryKeys.auth)).toEqual(student)
  expect(client.getQueryData(userQueryKeys.adminAuth)).toEqual(admin)
  window.history.replaceState({}, '', '/')
  expect(isCurrentSession(client, studentToken)).toBe(true)
})
