import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import type { User } from '../shared/domain'
import {
  createVariant,
  diagnosticApiBase,
  DiagnosticApiError,
  readDiagnostic,
  readDiagnosticResult,
  startDiagnostic,
  submitDiagnostic,
} from './diagnostic-api'
import type { DiagnosticSubmission } from './diagnostic-api'
import {
  loadDiagnosticIdentity,
  saveDiagnosticIdentity,
} from '../platform/diagnostic-session-storage'
import type { DiagnosticSessionIdentity } from '../platform/diagnostic-session-storage'
import { queryKeys } from './query-keys'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from './session-lifecycle'
import type { SessionToken } from './session-lifecycle'
import { acquireDiagnosticBootstrap } from './diagnostic-bootstrap'

function newIdentity(): DiagnosticSessionIdentity {
  return { variantKey: crypto.randomUUID(), startKey: crypto.randomUUID() }
}

async function startOperation(
  cache: QueryClient,
  userId: string,
  identity: DiagnosticSessionIdentity,
  signal: AbortSignal,
) {
  const token = captureSession(cache)
  const controller = new AbortController()
  const unregister = registerSessionRequest(cache, token, controller)
  const combined = AbortSignal.any([signal, controller.signal])
  function checkSession() {
    combined.throwIfAborted()
    if (!isCurrentSession(cache, token))
      throw new DOMException('Session changed', 'AbortError')
  }
  try {
    checkSession()
    const saved = loadDiagnosticIdentity(userId, diagnosticApiBase)
    let current = saved?.startKey === identity.startKey ? saved : identity
    if (!current.variantId) {
      const variant = await createVariant(current.variantKey, combined)
      checkSession()
      current = { ...current, variantId: variant.id }
      saveDiagnosticIdentity(userId, diagnosticApiBase, current)
    }
    const progress = await startDiagnostic(
      current.variantId!,
      current.startKey,
      combined,
    )
    checkSession()
    current = { ...current, sessionId: progress.session_id }
    saveDiagnosticIdentity(userId, diagnosticApiBase, current)
    return { progress, identity: current, token }
  } finally {
    unregister()
  }
}

export function useDiagnosticSession(userId: string) {
  const cache = useQueryClient()
  const [identity, setIdentity] = useState(
    () => loadDiagnosticIdentity(userId, diagnosticApiBase) ?? newIdentity(),
  )
  const [bootstrapError, setBootstrapError] = useState<Error | null>(null)
  const [sessionToken] = useState(() => captureSession(cache))
  const startController = useRef<AbortController | null>(null)
  const pendingRestart = useRef<DiagnosticSessionIdentity | null>(null)
  useEffect(() => () => startController.current?.abort(), [])
  const queryKey = queryKeys.diagnosticProgress(userId, identity.sessionId)
  const handleError = useCallback(
    (error: unknown, token: SessionToken = sessionToken) => {
      if (
        isCurrentSession(cache, token) &&
        cache.getQueryData<User>(queryKeys.auth)?.id === userId &&
        error instanceof DiagnosticApiError &&
        error.status === 401
      ) {
        void replaceSession(cache, null)
      }
    },
    [cache, sessionToken, userId],
  )
  const start = useMutation({
    mutationKey: queryKeys.startDiagnostic(userId),
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: ({
      identity: submittedIdentity,
      signal,
    }: {
      identity: DiagnosticSessionIdentity
      signal: AbortSignal
    }) => startOperation(cache, userId, submittedIdentity, signal),
    onSuccess: ({ progress, identity: next, token }) => {
      if (!isCurrentSession(cache, token)) return
      setIdentity(next)
      setBootstrapError(null)
      cache.setQueryData(
        queryKeys.diagnosticProgress(userId, next.sessionId),
        progress,
      )
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  useEffect(() => {
    if (!identity.sessionId)
      saveDiagnosticIdentity(userId, diagnosticApiBase, identity)
  }, [identity, userId])
  useEffect(() => {
    if (identity.sessionId) return
    const token = captureSession(cache)
    let active = true
    const acquired = acquireDiagnosticBootstrap(
      cache,
      token,
      identity,
      (signal, savedIdentity) =>
        start
          .mutateAsync({ identity: savedIdentity, signal })
          .then((result) => result.progress),
    )
    void acquired.promise
      .then((progress) => {
        if (!active || !isCurrentSession(cache, token)) return
        const next = loadDiagnosticIdentity(userId, diagnosticApiBase)
        if (next?.sessionId) setIdentity(next)
        cache.setQueryData(
          queryKeys.diagnosticProgress(userId, next?.sessionId),
          progress,
        )
        setBootstrapError(null)
      })
      .catch((error: unknown) => {
        if (!active || !isCurrentSession(cache, token)) return
        if (!(error instanceof DOMException && error.name === 'AbortError'))
          setBootstrapError(
            error instanceof Error
              ? error
              : new Error('Diagnostic start failed'),
          )
        handleError(error, token)
      })
    return () => {
      active = false
      acquired.release()
    }
  }, [cache, handleError, identity, start.mutateAsync, userId])
  // The epoch belongs to QueryClient; the public resource identity is user/session.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const progressQuery = useQuery({
    queryKey,
    enabled: !!identity.sessionId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      try {
        return await readDiagnostic(identity.sessionId!, signal)
      } catch (error) {
        handleError(error, token)
        throw error
      }
    },
    staleTime: Infinity,
    retry: false,
  })
  const submit = useMutation({
    mutationKey: queryKeys.submitDiagnostic(userId, identity.sessionId ?? ''),
    gcTime: 0,
    retry: false,
    onMutate: () => captureSession(cache),
    mutationFn: async (input: {
      submission: DiagnosticSubmission
      signal: AbortSignal
    }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        return await submitDiagnostic(
          input.submission,
          AbortSignal.any([input.signal, controller.signal]),
        )
      } finally {
        unregister()
      }
    },
    onSuccess: async (progress, _input, token) => {
      if (
        !isCurrentSession(cache, token) ||
        !identity.sessionId ||
        progress.session_id !== identity.sessionId
      )
        return
      await cache.cancelQueries({
        queryKey: queryKeys.diagnosticProgress(userId, identity.sessionId),
        exact: true,
      })
      if (!isCurrentSession(cache, token)) return
      cache.setQueryData(
        queryKeys.diagnosticProgress(userId, identity.sessionId),
        progress,
      )
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  const restart = useMutation({
    mutationKey: queryKeys.startDiagnostic(userId),
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async () => {
      const next = pendingRestart.current ?? newIdentity()
      pendingRestart.current = next
      const saved = loadDiagnosticIdentity(userId, diagnosticApiBase)
      if (saved?.startKey !== next.startKey)
        saveDiagnosticIdentity(userId, diagnosticApiBase, next)
      const controller = new AbortController()
      startController.current?.abort()
      startController.current = controller
      const result = await startOperation(
        cache,
        userId,
        next,
        controller.signal,
      )
      return { ...result, previousSessionId: identity.sessionId }
    },
    onSuccess: ({ progress, identity: next, previousSessionId, token }) => {
      if (!isCurrentSession(cache, token)) return
      pendingRestart.current = null
      setIdentity(next)
      setBootstrapError(null)
      cache.setQueryData(
        queryKeys.diagnosticProgress(userId, next.sessionId),
        progress,
      )
      if (previousSessionId && previousSessionId !== next.sessionId)
        cache.removeQueries({
          queryKey: queryKeys.diagnosticProgress(userId, previousSessionId),
          exact: true,
        })
      cache.removeQueries({ queryKey: queryKeys.diagnosticResults(userId) })
      cache.removeQueries({ queryKey: ['diagnostic-audio', userId] })
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  const data = progressQuery.data ?? start.data?.progress
  const loadError = identity.sessionId
    ? progressQuery.error
    : (start.error ?? bootstrapError)
  const query = {
    data,
    error: loadError,
    isError: !!loadError,
    isPending: identity.sessionId
      ? progressQuery.isPending
      : !data && !loadError,
    isFetching: progressQuery.isFetching || start.isPending,
    refetch: async () => {
      if (identity.sessionId) {
        const result = await progressQuery.refetch()
        return { data: result.data, error: result.error }
      }
      const controller = new AbortController()
      startController.current?.abort()
      startController.current = controller
      try {
        const result = await start.mutateAsync({
          identity:
            loadDiagnosticIdentity(userId, diagnosticApiBase) ?? identity,
          signal: controller.signal,
        })
        return { data: result.progress, error: null }
      } catch (error) {
        return { data: undefined, error: error as Error }
      }
    },
  }
  return { query, submit, restart, handleError }
}
export function useDiagnosticResultQuery(userId: string, sessionId: string) {
  return useQuery({
    queryKey: queryKeys.diagnosticResult(userId, sessionId),
    queryFn: ({ signal }) => readDiagnosticResult(sessionId, signal),
    retry: false,
    staleTime: Infinity,
  })
}
