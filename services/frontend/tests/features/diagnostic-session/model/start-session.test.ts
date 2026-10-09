import { expect, it, vi } from 'vitest'
import { startDiagnosticSession } from '@/features/diagnostic-session/model/start-session'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session/model/session-identity'

const progress: DiagnosticProgress = {
  session_id: 'session-1',
  status: 'active',
  completed_tasks: 0,
  skipped_tasks: 0,
  total_tasks: 1,
}
const token = { userId: 'user-1', epoch: 3 }
const identity: DiagnosticSessionIdentity = {
  subjectId: 'subject:a',
  variantKey: 'variant-key',
  startKey: 'start-key',
}

it('resumes a saved variant and persists the returned session identity', async () => {
  const saved = { ...identity, variantId: 'variant-1' }
  const createVariant = vi.fn()
  const startDiagnostic = vi.fn(async () => progress)
  const save = vi.fn()
  const result = await startDiagnosticSession(
    'user-1',
    identity,
    new AbortController().signal,
    {
      apiBase: '/api/v1',
      api: { createVariant, startDiagnostic },
      sessions: {
        capture: () => token,
        isCurrent: () => true,
        register: () => () => undefined,
      },
      storage: { load: () => saved, save },
    },
  )

  expect(createVariant).not.toHaveBeenCalled()
  expect(startDiagnostic).toHaveBeenCalledWith(
    'variant-1',
    identity.startKey,
    expect.any(AbortSignal),
  )
  expect(result).toEqual({
    progress,
    identity: { ...saved, sessionId: 'session-1' },
    token,
  })
  expect(save).toHaveBeenCalledWith('user-1', '/api/v1', identity.subjectId, {
    ...saved,
    sessionId: 'session-1',
  })
})

it('does not write the session ID when the session changes during start', async () => {
  let current = true
  let resolveStart!: (value: DiagnosticProgress) => void
  const start = new Promise<DiagnosticProgress>((resolve) => {
    resolveStart = resolve
  })
  const save = vi.fn()
  const request = startDiagnosticSession(
    'user-1',
    identity,
    new AbortController().signal,
    {
      apiBase: '/api/v1',
      api: {
        createVariant: async () => ({ id: 'variant-1' }),
        startDiagnostic: () => start,
      },
      sessions: {
        capture: () => token,
        isCurrent: () => current,
        register: () => () => undefined,
      },
      storage: { load: () => undefined, save },
    },
  )

  await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(1))
  current = false
  resolveStart(progress)

  await expect(request).rejects.toMatchObject({ name: 'AbortError' })
  expect(save).toHaveBeenCalledTimes(1)
  expect(save).toHaveBeenCalledWith('user-1', '/api/v1', identity.subjectId, {
    ...identity,
    variantId: 'variant-1',
  })
})

it('combines caller cancellation and ignores a late variant response', async () => {
  let resolveVariant!: (value: { id: string }) => void
  let operationSignal!: AbortSignal
  const variant = new Promise<{ id: string }>((resolve) => {
    resolveVariant = resolve
  })
  const caller = new AbortController()
  const save = vi.fn()
  const request = startDiagnosticSession('user-1', identity, caller.signal, {
    apiBase: '/api/v1',
    api: {
      createVariant: (_subject, _key, signal) => {
        operationSignal = signal
        return variant
      },
      startDiagnostic: vi.fn(async () => progress),
    },
    sessions: {
      capture: () => token,
      isCurrent: () => true,
      register: () => () => undefined,
    },
    storage: { load: () => undefined, save },
  })

  await vi.waitFor(() => expect(operationSignal).toBeDefined())
  caller.abort()
  expect(operationSignal.aborted).toBe(true)
  resolveVariant({ id: 'variant-late' })

  await expect(request).rejects.toMatchObject({ name: 'AbortError' })
  expect(save).not.toHaveBeenCalled()
})
