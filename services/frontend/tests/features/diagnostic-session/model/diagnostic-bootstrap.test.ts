import { describe, expect, it, vi } from 'vitest'
import { acquireDiagnosticBootstrap } from '@/features/diagnostic-session/model/diagnostic-bootstrap'

const identity = { subjectId: 'subject:a', variantKey: 'v', startKey: 's' }
const progress = {
  session_id: 'session',
  status: 'active' as const,
  completed_tasks: 0,
  skipped_tasks: 0,
  total_tasks: 1,
}

describe('diagnostic bootstrap', () => {
  it('shares one pending start across consumers and survives a StrictMode cleanup/setup', async () => {
    const client = {} as import('@tanstack/react-query').QueryClient
    const run = vi.fn(async () => progress)
    const token = { userId: 'u', epoch: 1 }
    const first = acquireDiagnosticBootstrap(client, token, identity, run)
    first.release()
    const second = acquireDiagnosticBootstrap(client, token, identity, run)
    await expect(second.promise).resolves.toEqual(progress)
    expect(run).toHaveBeenCalledTimes(1)
    second.release()
  })
})
