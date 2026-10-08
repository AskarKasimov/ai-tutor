import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { userQueryKeys } from '@/entities/user'
import type { User } from '@/entities/user'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import type { DiagnosticSubmission } from '@/entities/diagnostic-session'
import { startDiagnosticSession } from './start-session'
import type { DiagnosticDependencies } from './dependencies-context'
import { useDiagnosticDependencies } from './dependencies-context'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'
import { diagnosticSessionQueryKeys } from './query-keys'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import type { SessionToken } from '@/entities/user'
import { acquireDiagnosticBootstrap } from './diagnostic-bootstrap'

async function startOperation(
  cache: QueryClient,
  userId: string,
  identity: DiagnosticSessionIdentity,
  signal: AbortSignal,
  dependencies: DiagnosticDependencies,
) {
  return startDiagnosticSession(userId, identity, signal, {
    apiBase: dependencies.apiBase,
    api: dependencies.diagnostic,
    sessions: {
      capture: () => captureSession(cache),
      isCurrent: (token) => isCurrentSession(cache, token),
      register: (token, controller) =>
        registerSessionRequest(cache, token, controller),
    },
    storage: dependencies.diagnosticStorage,
  })
}

export function useDiagnosticSession(userId: string) {
  const dependencies = useDiagnosticDependencies()
  const cache = useQueryClient()
  const [identity, setIdentity] = useState(
    () =>
      dependencies.diagnosticStorage.load(userId, dependencies.apiBase) ??
      dependencies.createDiagnosticIdentity(),
  )
  const [bootstrapError, setBootstrapError] = useState<Error | null>(null)
  const [sessionToken] = useState(() => captureSession(cache))
  const startController = useRef<AbortController | null>(null)
  const pendingRestart = useRef<DiagnosticSessionIdentity | null>(null)
  useEffect(() => () => startController.current?.abort(), [])
  const queryKey = diagnosticSessionQueryKeys.diagnosticProgress(
    userId,
    identity.sessionId,
  )
  const handleError = useCallback(
    (error: unknown, token: SessionToken = sessionToken) => {
      if (
        isCurrentSession(cache, token) &&
        cache.getQueryData<User>(userQueryKeys.auth)?.id === userId &&
        error instanceof DiagnosticApiError &&
        error.status === 401
      ) {
        void replaceSession(cache, null)
      }
    },
    [cache, sessionToken, userId],
  )
  const start = useMutation({
    mutationKey: diagnosticSessionQueryKeys.startDiagnostic(userId),
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: ({
      identity: submittedIdentity,
      signal,
    }: {
      identity: DiagnosticSessionIdentity
      signal: AbortSignal
    }) =>
      startOperation(cache, userId, submittedIdentity, signal, dependencies),
    onSuccess: ({ progress, identity: next, token }) => {
      if (!isCurrentSession(cache, token)) return
      setIdentity(next)
      setBootstrapError(null)
      cache.setQueryData(
        diagnosticSessionQueryKeys.diagnosticProgress(userId, next.sessionId),
        progress,
      )
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  useEffect(() => {
    if (!identity.sessionId)
      dependencies.diagnosticStorage.save(
        userId,
        dependencies.apiBase,
        identity,
      )
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
        const next = dependencies.diagnosticStorage.load(
          userId,
          dependencies.apiBase,
        )
        if (next?.sessionId) setIdentity(next)
        cache.setQueryData(
          diagnosticSessionQueryKeys.diagnosticProgress(
            userId,
            next?.sessionId,
          ),
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
        return await dependencies.diagnostic.readDiagnostic(
          identity.sessionId!,
          signal,
        )
      } catch (error) {
        handleError(error, token)
        throw error
      }
    },
    staleTime: Infinity,
    retry: false,
  })
  const submit = useMutation({
    mutationKey: diagnosticSessionQueryKeys.submitDiagnostic(
      userId,
      identity.sessionId ?? '',
    ),
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
        return await dependencies.diagnostic.submitDiagnostic(
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
        queryKey: diagnosticSessionQueryKeys.diagnosticProgress(
          userId,
          identity.sessionId,
        ),
        exact: true,
      })
      if (!isCurrentSession(cache, token)) return
      cache.setQueryData(
        diagnosticSessionQueryKeys.diagnosticProgress(
          userId,
          identity.sessionId,
        ),
        progress,
      )
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  const restart = useMutation({
    mutationKey: diagnosticSessionQueryKeys.startDiagnostic(userId),
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async () => {
      const next =
        pendingRestart.current ?? dependencies.createDiagnosticIdentity()
      pendingRestart.current = next
      const saved = dependencies.diagnosticStorage.load(
        userId,
        dependencies.apiBase,
      )
      if (saved?.startKey !== next.startKey)
        dependencies.diagnosticStorage.save(userId, dependencies.apiBase, next)
      const controller = new AbortController()
      startController.current?.abort()
      startController.current = controller
      const result = await startOperation(
        cache,
        userId,
        next,
        controller.signal,
        dependencies,
      )
      return { ...result, previousSessionId: identity.sessionId }
    },
    onSuccess: ({ progress, identity: next, previousSessionId, token }) => {
      if (!isCurrentSession(cache, token)) return
      pendingRestart.current = null
      setIdentity(next)
      setBootstrapError(null)
      cache.setQueryData(
        diagnosticSessionQueryKeys.diagnosticProgress(userId, next.sessionId),
        progress,
      )
      if (previousSessionId && previousSessionId !== next.sessionId)
        cache.removeQueries({
          queryKey: diagnosticSessionQueryKeys.diagnosticProgress(
            userId,
            previousSessionId,
          ),
          exact: true,
        })
      cache.removeQueries({
        queryKey: diagnosticSessionQueryKeys.diagnosticResults(userId),
      })
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
            dependencies.diagnosticStorage.load(userId, dependencies.apiBase) ??
            identity,
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
  const dependencies = useDiagnosticDependencies()
  return useQuery({
    queryKey: diagnosticSessionQueryKeys.diagnosticResult(userId, sessionId),
    queryFn: ({ signal }) =>
      dependencies.diagnostic.readDiagnosticResult(sessionId, signal),
    retry: false,
    staleTime: Infinity,
  })
}
