import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '../app/providers'
import * as api from './auth-api'
import { useAuthenticateMutation, useCurrentUserQuery } from './use-auth'
import { replaceSession } from './session-lifecycle'
import { queryKeys } from './query-keys'
import type { User } from '../shared/domain'

afterEach(() => vi.restoreAllMocks())
it('does not adopt a stale login after another login updates the operation token', async () => {
  const client = createQueryClient()
  const current: User = {
    id: 'current',
    email: 'current@example.com',
    display_name: null,
    role: 'student',
  }
  const stale: User = { ...current, id: 'stale' }
  const latest: User = { ...current, id: 'latest' }
  const callbacks: ((user: User) => void)[] = []
  vi.spyOn(api, 'authenticate').mockImplementation(
    () => new Promise<User>((resolve) => callbacks.push(resolve)),
  )
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  const view = renderHook(useAuthenticateMutation, { wrapper })
  let first!: Promise<unknown>
  let second!: Promise<unknown>
  await act(async () => {
    first = view.result.current.mutateAsync({
      mode: 'login',
      email: stale.email,
      password: 'first',
    })
  })
  await act(async () => {
    await replaceSession(client, current)
  })
  await act(async () => {
    second = view.result.current.mutateAsync({
      mode: 'login',
      email: latest.email,
      password: 'second',
    })
  })
  await act(async () => {
    callbacks[0](stale)
    await first
  })
  expect(client.getQueryData(queryKeys.auth)).toEqual(current)
  await act(async () => {
    callbacks[1](latest)
    await second
  })
  expect(client.getQueryData(queryKeys.auth)).toEqual(latest)
})

it('keeps logout when an earlier auth read finishes adopting its user', async () => {
  const client = createQueryClient()
  const user: User = {
    id: 'student',
    email: 'student@example.com',
    display_name: null,
    role: 'student',
  }
  vi.spyOn(api, 'readCurrentUser').mockResolvedValue(user)
  const cancelQueries = client.cancelQueries.bind(client)
  let release!: () => void
  let cancellationStarted = false
  vi.spyOn(client, 'cancelQueries').mockImplementation(async (...args) => {
    await cancelQueries(...args)
    if (!cancellationStarted) {
      cancellationStarted = true
      await new Promise<void>((resolve) => {
        release = resolve
      })
    }
  })
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  const view = renderHook(useCurrentUserQuery, { wrapper })
  await waitFor(() => expect(cancellationStarted).toBe(true))
  await act(async () => {
    await replaceSession(client, null)
    release()
  })
  await waitFor(() => expect(view.result.current.isFetching).toBe(false))
  expect(client.getQueryData(queryKeys.auth)).toBeNull()
})
