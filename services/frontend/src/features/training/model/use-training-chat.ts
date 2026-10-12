import { useEffect, useRef } from 'react'
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { InfiniteData, Query, QueryClient } from '@tanstack/react-query'
import {
  TrainingApiError,
  type TrainingChatPage,
  type TrainingProgress,
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

async function registered<T>(
  cache: QueryClient,
  token: SessionToken,
  signal: AbortSignal,
  call: (signal: AbortSignal) => Promise<T>,
) {
  const controller = new AbortController()
  const unregister = registerSessionRequest(cache, token, controller)
  try {
    const value = await call(AbortSignal.any([signal, controller.signal]))
    if (!isCurrentSession(cache, token))
      throw new DOMException('Session changed', 'AbortError')
    return value
  } finally {
    unregister()
  }
}

function unauthorized(
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

export function useTrainingChat(
  userId: string,
  sessionId: string,
  exerciseId: string,
) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  const queryKey = trainingQueryKeys.chat(userId, sessionId, exerciseId)
  const identity = `${userId}:${sessionId}:${exerciseId}`
  const controllers = useRef(new Set<AbortController>())
  const nextKey = useRef<{
    exerciseId: string
    revision: number
    key: string
  } | null>(null)
  const requestKeys = useRef(new Map<string, string>())
  const audioIds = useRef(new WeakMap<Blob, string>())
  const audioSequence = useRef(0)
  useEffect(
    () => () => {
      for (const controller of controllers.current) controller.abort()
      controllers.current.clear()
    },
    [identity],
  )

  // The auth registry is client scoped and must not become part of the resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const query = useInfiniteQuery({
    queryKey,
    enabled: !!userId && !!sessionId && !!exerciseId,
    retry: false,
    refetchInterval: (
      current: Query<
        TrainingChatPage,
        Error,
        InfiniteData<TrainingChatPage, string | undefined>,
        typeof queryKey
      >,
    ) =>
      current.state.data?.pages[0]?.pending_turn?.status === 'processing'
        ? 2000
        : false,
    refetchIntervalInBackground: false,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => {
      const token = captureSession(cache)
      try {
        return await registered(cache, token, signal, (requestSignal) =>
          training.readTrainingChat(
            sessionId,
            exerciseId,
            pageParam,
            40,
            requestSignal,
          ),
        )
      } catch (error) {
        unauthorized(cache, userId, token, error)
        throw error
      }
    },
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  })

  const execute = useMutation({
    mutationKey: ['training-chat-send', userId, sessionId, exerciseId],
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async (input: {
      kind: 'message' | 'check'
      source: 'text' | 'voice'
      revision: number
      text?: string
      audio?: Blob
    }) => {
      const token = captureSession(cache)
      let audioId = ''
      if (input.audio) {
        audioId =
          audioIds.current.get(input.audio) ??
          `audio-${++audioSequence.current}`
        audioIds.current.set(input.audio, audioId)
      }
      const fingerprint = JSON.stringify([
        input.kind,
        input.source,
        input.revision,
        input.text ?? '',
        audioId,
      ])
      const key = requestKeys.current.get(fingerprint) ?? crypto.randomUUID()
      requestKeys.current.set(fingerprint, key)
      const controller = new AbortController()
      controllers.current.add(controller)
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const signal = controller.signal
        const response =
          input.source === 'text'
            ? await training.sendTrainingChatText(
                sessionId,
                exerciseId,
                training.createTrainingChatText(
                  key,
                  input.revision,
                  input.kind,
                  input.text ?? '',
                ),
                signal,
              )
            : await training.sendTrainingChatVoice(
                sessionId,
                exerciseId,
                training.createTrainingChatVoice(
                  key,
                  input.revision,
                  input.kind,
                  input.audio ?? new Blob(),
                ),
                signal,
              )
        if (!isCurrentSession(cache, token))
          throw new DOMException('Session changed', 'AbortError')
        return { response, token, fingerprint }
      } catch (error) {
        unauthorized(cache, userId, token, error)
        throw error
      } finally {
        unregister()
        controllers.current.delete(controller)
      }
    },
    onSuccess: async ({ response, token, fingerprint }) => {
      requestKeys.current.delete(fingerprint)
      if (
        !isCurrentSession(cache, token) ||
        response.session_id !== sessionId ||
        response.exercise_id !== exerciseId
      )
        return
      await cache.cancelQueries({ queryKey, exact: true })
      if (!isCurrentSession(cache, token)) return
      cache.setQueryData(
        queryKey,
        (
          current:
            | { pages: TrainingChatPage[]; pageParams: (string | undefined)[] }
            | undefined,
        ) => {
          if (!current?.pages.length) return current
          const pages = [...current.pages]
          const first = pages[0]!
          const merged = new Map(first.items.map((item) => [item.id, item]))
          for (const item of response.messages) merged.set(item.id, item)
          pages[0] = {
            ...first,
            revision: response.revision,
            latest_check: response.latest_check,
            pending_turn: null,
            items: [...merged.values()].sort((a, b) => a.sequence - b.sequence),
          }
          return { ...current, pages }
        },
      )
      void cache.invalidateQueries({ queryKey, exact: true })
    },
    onError: (error, _input, token) => {
      if (token) unauthorized(cache, userId, token, error)
      void cache.invalidateQueries({ queryKey, exact: true })
    },
  })

  const retry = useMutation({
    mutationKey: ['training-chat-retry', userId, sessionId, exerciseId],
    retry: false,
    gcTime: 0,
    mutationFn: async (turnId: string) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      controllers.current.add(controller)
      try {
        const response = await registered(
          cache,
          token,
          controller.signal,
          (signal) =>
            training.retryTrainingChatTurn(
              sessionId,
              exerciseId,
              turnId,
              signal,
            ),
        )
        if (!isCurrentSession(cache, token))
          throw new DOMException('Session changed', 'AbortError')
        return { response, token }
      } catch (error) {
        unauthorized(cache, userId, token, error)
        throw error
      } finally {
        controllers.current.delete(controller)
      }
    },
    onSuccess: async ({ response, token }) => {
      if (
        !isCurrentSession(cache, token) ||
        response.session_id !== sessionId ||
        response.exercise_id !== exerciseId
      )
        return
      await cache.invalidateQueries({ queryKey, exact: true })
    },
  })

  const cancel = useMutation({
    mutationKey: ['training-chat-cancel', userId, sessionId, exerciseId],
    retry: false,
    gcTime: 0,
    mutationFn: async (turnId: string) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      controllers.current.add(controller)
      try {
        await registered(cache, token, controller.signal, (signal) =>
          training.cancelTrainingChatTurn(
            sessionId,
            exerciseId,
            turnId,
            signal,
          ),
        )
        return token
      } catch (error) {
        unauthorized(cache, userId, token, error)
        throw error
      } finally {
        controllers.current.delete(controller)
      }
    },
    onSuccess: async (token) => {
      if (isCurrentSession(cache, token))
        await cache.invalidateQueries({ queryKey, exact: true })
    },
  })

  const next = useMutation({
    mutationKey: ['training-chat-next', userId, sessionId, exerciseId],
    retry: false,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async (revision: number) => {
      const token = captureSession(cache)
      const pending =
        nextKey.current?.exerciseId === exerciseId &&
        nextKey.current.revision === revision
          ? nextKey.current
          : { exerciseId, revision, key: crypto.randomUUID() }
      nextKey.current = pending
      const controller = new AbortController()
      controllers.current.add(controller)
      try {
        const progress = await registered(
          cache,
          token,
          controller.signal,
          (signal) =>
            training.nextTrainingExercise(
              sessionId,
              exerciseId,
              revision,
              pending.key,
              signal,
            ),
        )
        return { progress, token }
      } catch (error) {
        unauthorized(cache, userId, token, error)
        throw error
      } finally {
        controllers.current.delete(controller)
      }
    },
    onSuccess: async ({ progress, token }) => {
      if (
        !isCurrentSession(cache, token) ||
        progress.session_id !== sessionId ||
        progress.answer?.exercise_id !== exerciseId
      )
        return
      nextKey.current = null
      await cache.cancelQueries({
        queryKey: trainingQueryKeys.session(userId, sessionId),
        exact: true,
      })
      if (!isCurrentSession(cache, token)) return
      cache.setQueryData<TrainingProgress>(
        trainingQueryKeys.session(userId, sessionId),
        progress,
      )
      await Promise.all([
        cache.invalidateQueries({ queryKey, exact: true }),
        cache.invalidateQueries({
          queryKey: trainingQueryKeys.history(userId, sessionId),
        }),
      ])
    },
    onError: (error, _revision, token) => {
      if (token) unauthorized(cache, userId, token, error)
      void cache.invalidateQueries({ queryKey, exact: true })
      void cache.invalidateQueries({
        queryKey: trainingQueryKeys.session(userId, sessionId),
        exact: true,
      })
    },
  })

  return { query, send: execute, retry, cancel, next }
}
