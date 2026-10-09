import { useEffect, useRef } from 'react'
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import {
  TrainingApiError,
  type TrainingPreview,
  type TrainingProgress,
  type TrainingSubmission,
} from '@/entities/training'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
  userQueryKeys,
} from '@/entities/user'
import type { SessionToken } from '@/entities/user'
import { useTrainingDependencies } from './dependencies-context'
import { trainingQueryKeys } from './query-keys'

type Entry =
  | { kind: 'existing'; progress: TrainingProgress }
  | { kind: 'preview'; preview: TrainingPreview }

async function registered<T>(
  cache: QueryClient,
  token: SessionToken,
  signal: AbortSignal,
  call: (signal: AbortSignal) => Promise<T>,
) {
  const controller = new AbortController()
  const unregister = registerSessionRequest(cache, token, controller)
  const requestSignal = AbortSignal.any([signal, controller.signal])
  try {
    const value = await call(requestSignal)
    if (!isCurrentSession(cache, token))
      throw new DOMException('Session changed', 'AbortError')
    return { value, token }
  } finally {
    unregister()
  }
}

function handleUnauthorized(
  cache: QueryClient,
  userId: string,
  token: SessionToken,
  error: unknown,
) {
  if (
    isCurrentSession(cache, token) &&
    cache.getQueryData<{ id: string } | null>(userQueryKeys.auth)?.id ===
      userId &&
    error instanceof TrainingApiError &&
    error.status === 401
  )
    void replaceSession(cache, null)
}

export function useTrainingEntryQuery(userId: string, diagnosticId: string) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  // The auth registry is client scoped and must not become part of the resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery<Entry>({
    queryKey: trainingQueryKeys.entry(userId, diagnosticId),
    enabled: !!userId && !!diagnosticId,
    retry: false,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      try {
        try {
          const existing = await registered(
            cache,
            token,
            signal,
            (requestSignal) =>
              training.findTrainingForDiagnostic(diagnosticId, requestSignal),
          )
          return { kind: 'existing', progress: existing.value }
        } catch (error) {
          if (!(error instanceof TrainingApiError && error.status === 404))
            throw error
        }
        const preview = await registered(
          cache,
          token,
          signal,
          (requestSignal) =>
            training.readTrainingPreview(diagnosticId, requestSignal),
        )
        return { kind: 'preview', preview: preview.value }
      } catch (error) {
        handleUnauthorized(cache, userId, token, error)
        throw error
      }
    },
  })
}

export function useStartTrainingMutation(userId: string, diagnosticId: string) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  const startKey = useRef<{ identity: string; key: string } | null>(null)
  const request = useRef<AbortController | null>(null)
  const identity = `${userId}:${diagnosticId}`
  useEffect(() => () => request.current?.abort(), [identity])
  if (startKey.current?.identity !== identity) startKey.current = null
  return useMutation({
    mutationKey: ['training-start', userId, diagnosticId],
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async (input: { preview: TrainingPreview }) => {
      const token = captureSession(cache)
      const pending = startKey.current ?? { identity, key: crypto.randomUUID() }
      startKey.current = pending
      const controller = new AbortController()
      request.current?.abort()
      request.current = controller
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const progress = await training.startTraining(
          diagnosticId,
          pending.key,
          input.preview.mode === 'free_practice'
            ? input.preview.plan_revision
            : undefined,
          controller.signal,
        )
        return { progress, token, key: pending.key }
      } catch (error) {
        handleUnauthorized(cache, userId, token, error)
        throw error
      } finally {
        unregister()
        if (request.current === controller) request.current = null
      }
    },
    onSuccess: async ({ progress, token }) => {
      if (
        !isCurrentSession(cache, token) ||
        progress.diagnostic_session_id !== diagnosticId
      )
        return
      startKey.current = null
      cache.setQueryData(
        trainingQueryKeys.session(userId, progress.session_id),
        progress,
      )
      await cache.invalidateQueries({
        queryKey: trainingQueryKeys.entry(userId, diagnosticId),
      })
    },
    onError: async (error, _input, token) => {
      if (token) handleUnauthorized(cache, userId, token, error)
      if (
        token &&
        isCurrentSession(cache, token) &&
        error instanceof TrainingApiError &&
        error.status === 409
      ) {
        startKey.current = null
        await cache.invalidateQueries({
          queryKey: trainingQueryKeys.entry(userId, diagnosticId),
        })
      }
    },
  })
}

export function useTrainingSession(userId: string, sessionId: string) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  const queryKey = trainingQueryKeys.session(userId, sessionId)
  const request = useRef<AbortController | null>(null)
  useEffect(() => () => request.current?.abort(), [userId, sessionId])
  // The auth registry is client scoped and must not become part of the resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const query = useQuery({
    queryKey,
    enabled: !!userId && !!sessionId,
    retry: false,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      try {
        return (
          await registered(cache, token, signal, (requestSignal) =>
            training.readTrainingSession(sessionId, requestSignal),
          )
        ).value
      } catch (error) {
        handleUnauthorized(cache, userId, token, error)
        throw error
      }
    },
  })
  const submit = useMutation({
    mutationKey: ['training-submit', userId, sessionId],
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async (input: {
      submission: TrainingSubmission
      signal: AbortSignal
    }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      request.current?.abort()
      request.current = controller
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const progress = await training.submitTraining(
          input.submission,
          AbortSignal.any([input.signal, controller.signal]),
        )
        return { progress, token }
      } catch (error) {
        handleUnauthorized(cache, userId, token, error)
        throw error
      } finally {
        unregister()
        if (request.current === controller) request.current = null
      }
    },
    onSuccess: async ({ progress, token }, input) => {
      if (
        !isCurrentSession(cache, token) ||
        progress.session_id !== sessionId ||
        progress.answer?.exercise_id !== input.submission.exerciseId
      )
        return
      await cache.cancelQueries({ queryKey, exact: true })
      if (!isCurrentSession(cache, token)) return
      cache.setQueryData(queryKey, progress)
      void cache.invalidateQueries({
        queryKey: trainingQueryKeys.history(userId, sessionId),
      })
    },
    onError: (error, _input, token) => {
      if (token) handleUnauthorized(cache, userId, token, error)
    },
  })
  return { query, submit }
}

export function useTrainingHistoryQuery(userId: string, sessionId: string) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  // The auth registry is client scoped and must not become part of the resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useInfiniteQuery({
    queryKey: trainingQueryKeys.history(userId, sessionId),
    enabled: !!userId && !!sessionId,
    initialPageParam: undefined as string | undefined,
    retry: false,
    queryFn: async ({ pageParam, signal }) => {
      const token = captureSession(cache)
      try {
        return (
          await registered(cache, token, signal, (requestSignal) =>
            training.readTrainingHistory(
              sessionId,
              pageParam,
              20,
              requestSignal,
            ),
          )
        ).value
      } catch (error) {
        handleUnauthorized(cache, userId, token, error)
        throw error
      }
    },
    getNextPageParam: (page) => page.next_cursor,
  })
}
