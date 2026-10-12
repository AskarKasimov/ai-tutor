import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { LearningPlan } from '@/entities/learning-plan'
import { useLearningPlanDependencies } from './dependencies-context'
import { learningPlanQueryKeys } from './query-keys'

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
    mutationFn: (hours: number) =>
      learningPlan.createLearningPlan(
        diagnosticId,
        hours,
        new AbortController().signal,
      ),
    onSuccess: (plan) =>
      cache.setQueryData(
        learningPlanQueryKeys.byDiagnostic(userId, diagnosticId),
        plan,
      ),
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
    // While an answer is checked, its transcript is saved first: polling shows
    // it in the chat before the teacher replies.
    refetchInterval: polling ? 1500 : false,
    queryFn: ({ signal }) =>
      learningPlan.readLearningMessages(planId, index ?? 0, signal),
  })
}

// Lesson actions return the updated plan; the chat of the touched topics is
// read again.
export function useLessonActions(userId: string, plan: LearningPlan) {
  const { learningPlan } = useLearningPlanDependencies()
  const cache = useQueryClient()
  const touched = plan.current_index
  function settle(next: LearningPlan) {
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
  const signal = () => new AbortController().signal
  const open = useMutation({
    mutationFn: () => learningPlan.openLesson(plan.id, signal()),
    onSuccess: settle,
  })
  const answer = useMutation({
    mutationFn: (audio: Blob) =>
      learningPlan.answerLesson(plan.id, audio, signal()),
    onSuccess: settle,
    // A failed check still saved the transcript: show it with a retry.
    onError: () => {
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
    mutationFn: () => learningPlan.retryLessonAnswer(plan.id, signal()),
    onSuccess: settle,
  })
  return { open, answer, retry }
}
