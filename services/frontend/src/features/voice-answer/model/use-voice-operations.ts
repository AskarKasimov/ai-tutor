import { useMutation } from '@tanstack/react-query'
import { useVoiceAnswerDependencies } from './dependencies-context'
import type { AssessmentTask } from '@/entities/assessment'
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
export function useSynthesisMutation(userId: string) {
  const { voice } = useVoiceAnswerDependencies()
  return useMutation({
    mutationKey: voiceAnswerQueryKeys.synthesize(userId),
    mutationFn: ({ text, signal }: { text: string; signal: AbortSignal }) =>
      voice.synthesizeQuestion(text, signal),
    retry: false,
    gcTime: 0,
  })
}
export function useAssessmentMutation(userId: string) {
  const { assessment } = useVoiceAnswerDependencies()
  return useMutation({
    mutationKey: voiceAnswerQueryKeys.evaluate(userId),
    mutationFn: ({
      transcriptionId,
      task,
      signal,
    }: {
      transcriptionId: string
      task: AssessmentTask
      signal: AbortSignal
    }) => assessment.evaluateAnswer(transcriptionId, task, signal),
    retry: false,
    gcTime: 0,
  })
}
