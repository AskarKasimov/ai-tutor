import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  LearningPlan,
  ConfirmedLessonAnswer,
} from '@/entities/learning-plan'
import { LearningPlanApiError } from '@/entities/learning-plan'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import type { QueryClient } from '@tanstack/react-query'
import { useLearningPlanDependencies } from './dependencies-context'
import { learningPlanQueryKeys } from './query-keys'

async function scoped<T>(
  client: QueryClient,
  userId: string,
  signal: AbortSignal,
  request: (signal: AbortSignal) => Promise<T>,
) {
  const token = captureSession(client)
  if (token.userId !== userId || !isCurrentSession(client, token))
    throw new DOMException('Stale session', 'AbortError')
  const controller = new AbortController()
  const unregister = registerSessionRequest(client, token, controller)
  try {
    const value = await request(AbortSignal.any([signal, controller.signal]))
    if (!isCurrentSession(client, token) || token.userId !== userId)
      throw new DOMException('Stale session', 'AbortError')
    return value
  } catch (error) {
    if (
      error instanceof LearningPlanApiError &&
      error.status === 401 &&
      isCurrentSession(client, token)
    )
      await replaceSession(client, null)
    throw error
  } finally {
    unregister()
  }
}

// The plan of a diagnostic; null until the student has chosen their hours.
export function useLearningPlanQuery(userId: string, diagnosticId: string) {
  const { learningPlan } = useLearningPlanDependencies()
  return useQuery({
    queryKey: learningPlanQueryKeys.byDiagnostic(userId, diagnosticId),
    enabled: !!userId && !!diagnosticId,
    queryFn: ({ signal }) =>
      learningPlan.findLearningPlan(diagnosticId, signal),
  })
}

export function useCreateLearningPlan(userId: string, diagnosticId: string) {
  const { learningPlan } = useLearningPlanDependencies()
  const cache = useQueryClient()
  return useMutation({
    onMutate: () => captureSession(cache),
    mutationFn: (hours: number) =>
      scoped(cache, userId, new AbortController().signal, (signal) =>
        learningPlan.createLearningPlan(diagnosticId, hours, signal),
      ),
    onSuccess: (plan, _hours, token) => {
      if (token && isCurrentSession(cache, token) && token.userId === userId)
        cache.setQueryData(
          learningPlanQueryKeys.byDiagnostic(userId, diagnosticId),
          plan,
        )
    },
  })
}

export function useLessonMessages(
  userId: string,
  planId: string,
  index: number | null,
  polling: boolean,
) {
  const { learningPlan } = useLearningPlanDependencies()
  return useQuery({
    queryKey: learningPlanQueryKeys.messages(userId, planId, index ?? -1),
    enabled: !!planId && index !== null,
    // Theory and answer transcripts are saved before the full reply is ready.
    refetchInterval: polling ? 1500 : false,
    queryFn: ({ signal }) =>
      learningPlan.readLearningMessages(planId, index ?? 0, signal),
  })
}

// Lesson actions return the updated plan; the chat of the touched topics is
// read again. onChecked receives the plan before and after a checked answer.
export function useLessonActions(
  userId: string,
  plan: LearningPlan,
  onChecked?: (before: LearningPlan, after: LearningPlan) => void,
) {
  const { learningPlan } = useLearningPlanDependencies()
  const cache = useQueryClient()
  const touched = plan.current_index
  function settle(
    next: LearningPlan,
    token: ReturnType<typeof captureSession>,
  ) {
    if (!isCurrentSession(cache, token) || token.userId !== userId) return
    cache.setQueryData(
      learningPlanQueryKeys.byDiagnostic(userId, plan.diagnostic_session_id),
      next,
    )
    for (const index of new Set([touched, next.current_index]))
      if (index !== null)
        void cache.invalidateQueries({
          queryKey: learningPlanQueryKeys.messages(userId, plan.id, index),
        })
  }
  const open = useMutation({
    onMutate: () => captureSession(cache),
    mutationFn: () =>
      scoped(cache, userId, new AbortController().signal, (signal) =>
        learningPlan.openLesson(plan.id, signal),
      ),
    onSuccess: (next, _variables, token) => {
      if (token) settle(next, token)
    },
    // A failed practice request still leaves the theory available to read.
    onError: (_error, _variables, token) => {
      if (!token || !isCurrentSession(cache, token) || token.userId !== userId)
        return
      if (touched !== null)
        void cache.invalidateQueries({
          queryKey: learningPlanQueryKeys.messages(userId, plan.id, touched),
        })
    },
  })
  const answer = useMutation({
    onMutate: () => captureSession(cache),
    mutationFn: (audio: Blob | ConfirmedLessonAnswer) =>
      scoped(cache, userId, new AbortController().signal, (signal) =>
        learningPlan.answerLesson(plan.id, audio, signal),
      ),
    onSuccess: (next, _audio, token) => {
      if (!token || !isCurrentSession(cache, token) || token.userId !== userId)
        return
      settle(next, token)
      onChecked?.(plan, next)
    },
    // A failed check still saved the transcript: show it with a retry.
    onError: (_error, _audio, token) => {
      if (!token || !isCurrentSession(cache, token) || token.userId !== userId)
        return
      void cache.invalidateQueries({
        queryKey: learningPlanQueryKeys.byDiagnostic(
          userId,
          plan.diagnostic_session_id,
        ),
      })
      if (touched !== null)
        void cache.invalidateQueries({
          queryKey: learningPlanQueryKeys.messages(userId, plan.id, touched),
        })
    },
  })
  const retry = useMutation({
    onMutate: () => captureSession(cache),
    mutationFn: () =>
      scoped(cache, userId, new AbortController().signal, (signal) =>
        learningPlan.retryLessonAnswer(plan.id, signal),
      ),
    onSuccess: (next, _variables, token) => {
      if (!token || !isCurrentSession(cache, token) || token.userId !== userId)
        return
      settle(next, token)
      onChecked?.(plan, next)
    },
  })
  return { open, answer, retry }
}
