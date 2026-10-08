import { useMutation } from '@tanstack/react-query'
import { useVoiceAnswerDependencies } from './dependencies-context'
import type { OverallFeedbackItemInput } from '@/entities/assessment'
import { voiceAnswerQueryKeys } from './query-keys'

export function useOverallFeedbackMutation(userId: string) {
  const { assessment } = useVoiceAnswerDependencies()
  return useMutation({
    mutationKey: voiceAnswerQueryKeys.overallFeedback(userId),
    mutationFn: ({
      answers,
      signal,
    }: {
      answers: OverallFeedbackItemInput[]
      signal: AbortSignal
    }) => assessment.fetchOverallFeedback(answers, signal),
    retry: false,
    gcTime: 0,
  })
}
