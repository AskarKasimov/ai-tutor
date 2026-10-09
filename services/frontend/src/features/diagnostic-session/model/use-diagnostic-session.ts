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

export function useDiagnosticSession(
  userId: string,
  subjectId: string,
  initialSessionId?: string,
) {
  const dependencies = useDiagnosticDependencies()
  const cache = useQueryClient()
  const [identity, setIdentity] = useState(() =>
    initialSessionId
      ? {
          ...(dependencies.diagnosticStorage.load(
            userId,
            dependencies.apiBase,
            subjectId,
          ) ?? dependencies.createDiagnosticIdentity(subjectId)),
          sessionId: initialSessionId,
        }
      : (dependencies.diagnosticStorage.load(
          userId,
          dependencies.apiBase,
          subjectId,
        ) ?? dependencies.createDiagnosticIdentity(subjectId)),
  )
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
  useEffect(() => {
    if (identity.sessionId)
      dependencies.diagnosticStorage.save(
        userId,
        dependencies.apiBase,
        subjectId,
        identity,
      )
  }, [dependencies, identity, subjectId, userId])
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
        pendingRestart.current ??
        (!initialSessionId &&
        !identity.sessionId &&
        identity.subjectId === subjectId
          ? identity
          : dependencies.createDiagnosticIdentity(subjectId))
      pendingRestart.current = next
      const saved = dependencies.diagnosticStorage.load(
        userId,
        dependencies.apiBase,
        subjectId,
      )
      if (saved?.startKey !== next.startKey)
        dependencies.diagnosticStorage.save(
          userId,
          dependencies.apiBase,
          subjectId,
          next,
        )
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
      cache.removeQueries({ queryKey: ['stored-task-audio', userId] })
    },
    onError: (error, _input, token) => {
      if (token) handleError(error, token)
    },
  })
  const data = progressQuery.data
  const loadError = identity.sessionId ? progressQuery.error : restart.error
  const query = {
    data,
    error: loadError,
    isError: !!loadError,
    isPending: identity.sessionId
      ? progressQuery.isPending
      : !data && !loadError,
    isFetching: progressQuery.isFetching || restart.isPending,
    refetch: async () => {
      if (identity.sessionId) {
        const result = await progressQuery.refetch()
        return { data: result.data, error: result.error }
      }
      try {
        const result = await restart.mutateAsync()
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

export function useDiagnosticFeedbackQuery(userId: string, sessionId: string) {
  const dependencies = useDiagnosticDependencies()
  return useQuery({
    queryKey: diagnosticSessionQueryKeys.diagnosticFeedback(userId, sessionId),
    queryFn: ({ signal }) =>
      dependencies.diagnostic.readDiagnosticFeedback(sessionId, signal),
    retry: false,
    staleTime: Infinity,
  })
}
