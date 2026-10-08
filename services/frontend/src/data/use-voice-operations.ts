import { useMutation, useQuery } from '@tanstack/react-query'
import { diagnosticAudio } from './diagnostic-api'
import { evaluateAnswer } from './assessment-api'
import { synthesizeQuestion, transcribeRecording } from './voice-api'
import type { AssessmentTask } from '../shared/domain'
import { queryKeys } from './query-keys'

export function useTranscriptionMutation(userId: string) {
  return useMutation({
    mutationKey: queryKeys.transcribe(userId),
    mutationFn: ({ blob, signal }: { blob: Blob; signal: AbortSignal }) =>
      transcribeRecording(blob, signal),
    retry: false,
    gcTime: 0,
  })
}
export function useSynthesisMutation(userId: string) {
  return useMutation({
    mutationKey: queryKeys.synthesize(userId),
    mutationFn: ({ text, signal }: { text: string; signal: AbortSignal }) =>
      synthesizeQuestion(text, signal),
    retry: false,
    gcTime: 0,
  })
}
export function useAssessmentMutation(userId: string) {
  return useMutation({
    mutationKey: queryKeys.evaluate(userId),
    mutationFn: ({
      transcriptionId,
      task,
      signal,
    }: {
      transcriptionId: string
      task: AssessmentTask
      signal: AbortSignal
    }) => evaluateAnswer(transcriptionId, task, signal),
    retry: false,
    gcTime: 0,
  })
}
export function useDiagnosticAudioQuery(
  userId: string,
  sessionId: string,
  taskId: string | undefined,
) {
  return useQuery({
    queryKey: queryKeys.diagnosticAudio(userId, sessionId, taskId),
    queryFn: ({ signal }) => diagnosticAudio(sessionId, signal),
    enabled: false,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}
