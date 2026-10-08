import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  Assessment,
  DemoAssessmentTask,
  OverallFeedbackData,
  OverallFeedbackItemInput,
} from '@/entities/assessment'
export type VoiceAnswerDependencies = {
  voice: {
    transcribeRecording(
      blob: Blob,
      signal: AbortSignal,
    ): Promise<{ id: string; text: string }>
    synthesizeQuestion(text: string, signal: AbortSignal): Promise<Blob>
  }
  assessment: {
    evaluateDemoAnswer(
      transcriptionId: string,
      task: DemoAssessmentTask,
      signal: AbortSignal,
    ): Promise<Assessment>
    fetchOverallFeedback(
      answers: OverallFeedbackItemInput[],
      signal?: AbortSignal,
    ): Promise<OverallFeedbackData>
  }
}
const Context = createContext<VoiceAnswerDependencies | null>(null)
export function VoiceAnswerDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: VoiceAnswerDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useVoiceAnswerDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Voice answer dependencies provider is missing')
  return value
}
