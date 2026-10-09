import { afterEach, expect, it, vi } from 'vitest'
import {
  loadDiagnosticIdentity,
  saveDiagnosticIdentity,
} from '@/features/diagnostic-session/model/diagnostic-session-storage'

afterEach(() => {
  sessionStorage.clear()
  vi.restoreAllMocks()
})

it('ignores legacy identities and scopes persisted identities by subject', () => {
  sessionStorage.setItem(
    'ai-tutor:diagnostic:/api/v1:user-1',
    JSON.stringify({
      variantKey: 'old-v',
      startKey: 'old-s',
      sessionId: 'old-session',
    }),
  )
  expect(
    loadDiagnosticIdentity('user-1', '/api/v1', 'subject:a'),
  ).toBeUndefined()
  const identity = { subjectId: 'subject:a', variantKey: 'v', startKey: 's' }
  saveDiagnosticIdentity('user-1', '/api/v1', 'subject:a', identity)
  expect(loadDiagnosticIdentity('user-1', '/api/v1', 'subject:a')).toEqual(
    identity,
  )
  expect(
    loadDiagnosticIdentity('user-1', '/api/v1', 'subject:b'),
  ).toBeUndefined()
})
