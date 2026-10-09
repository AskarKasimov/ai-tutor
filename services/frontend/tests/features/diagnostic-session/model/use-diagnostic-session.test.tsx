import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createQueryWrapper } from '../../../support/query-wrapper'
import {
  loadPendingDiagnosticIdentity,
  saveDiagnosticIdentity,
  loadDiagnosticIdentity,
} from '@/features/diagnostic-session/model/diagnostic-session-storage'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'
import * as api from '@/entities/diagnostic-session'
import { diagnosticSessionQueryKeys } from '@/features/diagnostic-session/model/query-keys'
import { userQueryKeys } from '@/entities/user'
import { replaceSession } from '@/entities/user'
import { useDiagnosticSession } from '@/features/diagnostic-session'

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
  saveDiagnosticIdentity(user.id, api.diagnosticApiBase, 'subject:test', {
    subjectId: 'subject:test',
    variantKey: 'original-v',
    startKey: 'original-s',
    variantId: 'original',
    sessionId: 'old',
  })
  vi.spyOn(api, 'readDiagnostic').mockResolvedValue(completed)
  const wrapper = createQueryWrapper(client)
  const view = renderHook(
    () => useDiagnosticSession(user.id, 'subject:test', 'old'),
    { wrapper },
  )
  await waitFor(() => expect(view.result.current.query.data).toEqual(completed))
  return { ...view, client }
}
beforeEach(() => sessionStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('diagnostic recovery and auth isolation', () => {
  it('does not load or persist the previous session while an explicit new diagnostic starts', async () => {
    const read = vi.spyOn(api, 'readDiagnostic').mockResolvedValue(completed)
    const createVariant = vi
      .spyOn(api, 'createVariant')
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ id: 'fresh-variant' })
    vi.spyOn(api, 'startDiagnostic').mockResolvedValue(next)
    const client = createQueryClient()
    await replaceSession(client, user)
    saveDiagnosticIdentity(user.id, api.diagnosticApiBase, 'subject:test', {
      subjectId: 'subject:test',
      variantKey: 'old-v',
      startKey: 'old-s',
      variantId: 'old',
      sessionId: 'old',
    })
    const view = renderHook(
      () => useDiagnosticSession(user.id, 'subject:test', undefined, true),
      { wrapper: createQueryWrapper(client) },
    )
    expect(read).not.toHaveBeenCalled()
    expect(view.result.current.query.data).toBeUndefined()
    await act(async () => {
      await view.result.current.restart.mutateAsync().catch(() => undefined)
    })
    expect(createVariant).toHaveBeenCalledOnce()
    expect(
      loadDiagnosticIdentity(user.id, api.diagnosticApiBase, 'subject:test'),
    ).toMatchObject({ sessionId: 'old', startKey: 'old-s' })
    await waitFor(() => expect(view.result.current.query.isError).toBe(true))
    expect(view.result.current.query.error?.message).toBe('offline')
    await act(async () => {
      await view.result.current.restart.mutateAsync()
    })
    expect(createVariant.mock.calls[0][1]).toBe(createVariant.mock.calls[1][1])
    expect(
      loadDiagnosticIdentity(user.id, api.diagnosticApiBase, 'subject:test'),
    ).toMatchObject({ sessionId: 'new', variantId: 'fresh-variant' })
  })
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
    const failedIdentity = loadPendingDiagnosticIdentity(
      user.id,
      api.diagnosticApiBase,
      'subject:test',
    )
    expect(failedIdentity).toMatchObject({ variantId: 'new-variant' })
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
        diagnosticSessionQueryKeys.diagnosticProgress(user.id, 'old'),
        fresh,
      )
      await act(async () => {
        if (outcome === 'success') resolve({ ...completed, score: 0 })
        else reject(new api.DiagnosticApiError(401))
        await request
      })
      expect(view.client.getQueryData(userQueryKeys.auth)).toEqual(user)
      expect(
        view.client.getQueryData(
          diagnosticSessionQueryKeys.diagnosticProgress(user.id, 'old'),
        ),
      ).toEqual(fresh)
      expect(signal.aborted).toBe(true)
    },
  )
})
