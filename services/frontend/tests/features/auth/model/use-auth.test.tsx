import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createQueryWrapper } from '../../../support/query-wrapper'
import * as api from '@/entities/user'
import { useAuthenticateMutation, useCurrentUserQuery } from '@/features/auth'
import { replaceSession } from '@/entities/user'
import { userQueryKeys } from '@/entities/user'
import type { User } from '@/entities/user'

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
  const wrapper = createQueryWrapper(client)
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
  expect(client.getQueryData(userQueryKeys.auth)).toEqual(current)
  await act(async () => {
    callbacks[1](latest)
    await second
  })
  expect(client.getQueryData(userQueryKeys.auth)).toEqual(latest)
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
  const wrapper = createQueryWrapper(client)
  const view = renderHook(useCurrentUserQuery, { wrapper })
  await waitFor(() => expect(cancellationStarted).toBe(true))
  await act(async () => {
    await replaceSession(client, null)
    release()
  })
  await waitFor(() => expect(view.result.current.isFetching).toBe(false))
  expect(client.getQueryData(userQueryKeys.auth)).toBeNull()
})
