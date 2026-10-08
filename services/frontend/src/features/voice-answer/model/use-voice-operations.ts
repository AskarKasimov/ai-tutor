import { useMutation } from '@tanstack/react-query'
import { useVoiceAnswerDependencies } from './dependencies-context'
import type { DemoAssessmentTask } from '@/entities/assessment'
import { voiceAnswerQueryKeys } from './query-keys'

export function useTranscriptionMutation(userId: string) {
  const { voice } = useVoiceAnswerDependencies()
  return useMutation({
    mutationKey: voiceAnswerQueryKeys.transcribe(userId),
    mutationFn: ({ blob, signal }: { blob: Blob; signal: AbortSignal }) =>
      voice.transcribeRecording(blob, signal),
    retry: false,
    gcTime: 0,
  })
}
export function useDemoAssessmentMutation(userId: string) {
  const { assessment } = useVoiceAnswerDependencies()
  return useMutation({
    mutationKey: voiceAnswerQueryKeys.evaluate(userId),
    mutationFn: ({
      transcriptionId,
      task,
      signal,
    }: {
      transcriptionId: string
      task: DemoAssessmentTask
      signal: AbortSignal
    }) => assessment.evaluateDemoAnswer(transcriptionId, task, signal),
    retry: false,
    gcTime: 0,
  })
}
