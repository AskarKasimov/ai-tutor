import { act, renderHook } from '@testing-library/react'
import { expect, it } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import type { DiagnosticSession } from '@/entities/diagnostic-session'
import { replaceSession } from '@/entities/user'
import { useStartDiagnosticMutation } from '@/features/diagnostic-session'
import { createQueryWrapper } from '../../../support/query-wrapper'

const user = {
  id: 'u1',
  email: 'student@example.com',
  display_name: null,
  role: 'student' as const,
}

const session = (subjectId: string): DiagnosticSession => ({
  id: `session-${subjectId}`,
  subjectId,
  status: 'active',
  progressPercent: 0,
  currentTurnId: null,
  startedAt: 0,
  completedAt: null,
  turns: [],
})

async function setup() {
  const client = createQueryClient()
  await replaceSession(client, user)
  const dependencies = createAppDependencies()
  const calls: { subjectId: string; key: string }[] = []
  let failing = true
  dependencies.diagnostic.startDiagnostic = async (subjectId, key) => {
    calls.push({ subjectId, key })
    if (failing) throw new Error('lost response')
    return session(subjectId)
  }
  const hook = renderHook(
    ({ subjectId }) => useStartDiagnosticMutation('u1', subjectId),
    {
      initialProps: { subjectId: 'a' },
      wrapper: createQueryWrapper(client, dependencies),
    },
  )
  const start = (subjectId: string) => {
    hook.rerender({ subjectId })
    return act(async () => {
      await hook.result.current.mutateAsync().catch(() => undefined)
    })
  }
  return {
    calls,
    start,
    succeed: () => {
      failing = false
    },
    fail: () => {
      failing = true
    },
  }
}

it('uses a separate key after a failed start when the course changes', async () => {
  const { calls, start } = await setup()
  await start('a')
  await start('b')
  expect(calls.map((c) => c.subjectId)).toEqual(['a', 'b'])
  expect(calls[1].key).not.toBe(calls[0].key)
})

it('repeats the key of the same course after a failure', async () => {
  const { calls, start } = await setup()
  await start('a')
  await start('a')
  expect(calls[1].key).toBe(calls[0].key)
})

it('keeps the key of a course after visiting another one', async () => {
  const { calls, start } = await setup()
  await start('a')
  await start('b')
  await start('a')
  expect(calls[2].key).toBe(calls[0].key)
})

it('drops only the key of the course that started successfully', async () => {
  const { calls, start, succeed, fail } = await setup()
  await start('a')
  await start('b')
  succeed()
  await start('a')
  fail()
  await start('b')
  await start('a')
  expect(calls[3].key).toBe(calls[1].key)
  expect(calls[4].key).not.toBe(calls[0].key)
})
