import { useCallback } from 'react'
import type { User } from '../shared/domain'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createVariant, diagnosticApiBase, DiagnosticApiError, readDiagnostic, readDiagnosticResult, startDiagnostic, submitDiagnostic } from './diagnostic-api'
import type { DiagnosticSubmission } from './diagnostic-api'
import { loadDiagnosticIdentity, saveDiagnosticIdentity } from '../platform/diagnostic-session-storage'

export function useDiagnosticSession(userId: string) {
  const cache = useQueryClient()
  const queryKey = ['diagnostic-session', userId] as const
  const handleError = useCallback((error: unknown) => {
    if (cache.getQueryData<User>(['auth', 'me'])?.id === userId && error instanceof DiagnosticApiError && error.status === 401) {
      cache.setQueryData(['auth', 'me'], null)
      cache.removeQueries({ queryKey: ['diagnostic-session'] })
      cache.removeQueries({ queryKey: ['diagnostic-result'] })
    }
  }, [cache, userId])
  async function load(signal: AbortSignal, restartFrom?: string) {
    let identity = loadDiagnosticIdentity(userId, diagnosticApiBase)
    if (!identity || (restartFrom && identity.sessionId === restartFrom)) {
      identity = { variantKey: crypto.randomUUID(), startKey: crypto.randomUUID() }
      saveDiagnosticIdentity(userId, diagnosticApiBase, identity)
    }
    try {
      if (identity.sessionId) return await readDiagnostic(identity.sessionId, signal)
      if (!identity.variantId) {
        const variant = await createVariant(identity.variantKey, signal)
        identity = { ...identity, variantId: variant.id }
        saveDiagnosticIdentity(userId, diagnosticApiBase, identity)
      }
      const progress = await startDiagnostic(identity.variantId!, identity.startKey, signal)
      saveDiagnosticIdentity(userId, diagnosticApiBase, { ...identity, sessionId: progress.session_id })
      return progress
    } catch (error) { handleError(error); throw error }
  }
  const query = useQuery({ queryKey, queryFn: ({ signal }) => load(signal), staleTime: Infinity, retry: false })
  const submit = useMutation({
    mutationFn: (input: { submission: DiagnosticSubmission; signal: AbortSignal }) => submitDiagnostic(input.submission, input.signal),
    onSuccess: (progress) => {
      if (cache.getQueryData<User>(['auth', 'me'])?.id !== userId) return
      cache.setQueryData(queryKey, progress)
      void cache.invalidateQueries({ queryKey, refetchType: 'none' })
    },
    onError: handleError,
    retry: false,
  })
  const restart = useMutation({
    mutationFn: () => load(AbortSignal.timeout(120_000), query.data?.session_id ?? loadDiagnosticIdentity(userId, diagnosticApiBase)?.sessionId),
    onSuccess: (progress) => {
      if (cache.getQueryData<User>(['auth', 'me'])?.id !== userId) return
      cache.setQueryData(queryKey, progress)
      cache.removeQueries({ queryKey: ['diagnostic-result', userId] })
    },
    onError: handleError,
    retry: false,
  })
  return { query, submit, restart, handleError }
}
export function useDiagnosticResultQuery(userId: string, sessionId: string) {
  return useQuery({ queryKey: ['diagnostic-result', userId, sessionId], queryFn: ({ signal }) => readDiagnosticResult(sessionId, signal), retry: false, staleTime: Infinity })
}
