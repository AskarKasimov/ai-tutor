import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '../app/providers'
import {
  saveDiagnosticIdentity,
  loadDiagnosticIdentity,
} from '../platform/diagnostic-session-storage'
import type { DiagnosticProgress } from '../shared/domain'
import * as api from './diagnostic-api'
import { queryKeys } from './query-keys'
import { replaceSession } from './session-lifecycle'
import { useDiagnosticSession } from './use-diagnostic-session'

const user = {
  id: 'student',
  email: 'student@example.com',
  display_name: null,
  role: 'student' as const,
}
const completed: DiagnosticProgress = {
  session_id: 'old',
  status: 'completed',
  completed_tasks: 1,
  skipped_tasks: 0,
  total_tasks: 1,
}
const next: DiagnosticProgress = { ...completed, session_id: 'new' }
async function mount() {
  const client = createQueryClient()
  await replaceSession(client, user)
  saveDiagnosticIdentity(user.id, api.diagnosticApiBase, {
    variantKey: 'original-v',
    startKey: 'original-s',
    variantId: 'original',
    sessionId: 'old',
  })
  vi.spyOn(api, 'readDiagnostic').mockResolvedValue(completed)
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  const view = renderHook(() => useDiagnosticSession(user.id), { wrapper })
  await waitFor(() => expect(view.result.current.query.data).toEqual(completed))
  return { ...view, client }
}
beforeEach(() => sessionStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('diagnostic recovery and auth isolation', () => {
  it('keeps the completed result visible after a failed restart', async () => {
    vi.spyOn(api, 'createVariant').mockRejectedValue(new Error('Lost response'))
    const view = await mount()
    await act(async () => {
      await view.result.current.restart.mutateAsync().catch(() => undefined)
    })
    await waitFor(() => expect(view.result.current.restart.isError).toBe(true))
    expect(view.result.current.query.isError).toBe(false)
    expect(view.result.current.query.data).toEqual(completed)
  })
  it('retries the pending restart with the same identity and confirmed variant', async () => {
    const variants = vi
      .spyOn(api, 'createVariant')
      .mockResolvedValue({ id: 'new-variant' })
    const starts = vi
      .spyOn(api, 'startDiagnostic')
      .mockRejectedValueOnce(new Error('Lost start response'))
      .mockResolvedValue(next)
    const view = await mount()
    await act(async () => {
      await view.result.current.restart.mutateAsync().catch(() => undefined)
    })
    const failedIdentity = loadDiagnosticIdentity(
      user.id,
      api.diagnosticApiBase,
    )
    await act(async () => {
      await view.result.current.restart.mutateAsync()
    })
    expect(view.result.current.query.data).toEqual(next)
    expect(variants).toHaveBeenCalledTimes(1)
    expect(starts.mock.calls.map(([variant, key]) => [variant, key])).toEqual([
      ['new-variant', failedIdentity!.startKey],
      ['new-variant', failedIdentity!.startKey],
    ])
  })
  it.each(['success', '401'] as const)(
    'ignores a late submit %s after the same user logs in again',
    async (outcome) => {
      let resolve!: (value: DiagnosticProgress) => void
      let reject!: (error: Error) => void
      let signal!: AbortSignal
      vi.spyOn(api, 'submitDiagnostic').mockImplementation(
        (_input, inputSignal) => {
          signal = inputSignal
          return new Promise<DiagnosticProgress>((yes, no) => {
            resolve = yes
            reject = no
          })
        },
      )
      const view = await mount()
      let request!: Promise<unknown>
      await act(async () => {
        request = view.result.current.submit
          .mutateAsync({
            submission: {
              sessionId: 'old',
              taskId: 'task',
              role: 'main',
              key: 'answer',
              body: new FormData(),
            },
            signal: new AbortController().signal,
          })
          .catch(() => undefined)
      })
      await waitFor(() => expect(signal).toBeDefined())
      await act(async () => {
        await replaceSession(view.client, null)
        await replaceSession(view.client, user)
      })
      const fresh = { ...completed, score: 2 }
      view.client.setQueryData(
        queryKeys.diagnosticProgress(user.id, 'old'),
        fresh,
      )
      await act(async () => {
        if (outcome === 'success') resolve({ ...completed, score: 0 })
        else reject(new api.DiagnosticApiError(401))
        await request
      })
      expect(view.client.getQueryData(queryKeys.auth)).toEqual(user)
      expect(
        view.client.getQueryData(queryKeys.diagnosticProgress(user.id, 'old')),
      ).toEqual(fresh)
      expect(signal.aborted).toBe(true)
    },
  )
})
