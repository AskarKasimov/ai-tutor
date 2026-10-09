import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  DiagnosticProgress,
  DiagnosticAudioMetadata,
  DiagnosticResult,
  DiagnosticOverallFeedback,
  DiagnosticSubmission,
  DiagnosticTask,
  DiagnosticSessionIdentity,
} from '@/entities/diagnostic-session'
export type DiagnosticDependencies = {
  apiBase: string
  createDiagnosticIdentity(subjectId: string): DiagnosticSessionIdentity
  diagnosticStorage: {
    load(
      userId: string,
      apiBase: string,
      subjectId: string,
    ): DiagnosticSessionIdentity | undefined
    save(
      userId: string,
      apiBase: string,
      subjectId: string,
      identity: DiagnosticSessionIdentity,
    ): void
  }
  diagnostic: {
    createVariant(
      subjectId: string,
      key: string,
      signal: AbortSignal,
    ): Promise<{ id: string }>
    startDiagnostic(
      variantId: string,
      key: string,
      signal: AbortSignal,
    ): Promise<DiagnosticProgress>
    readDiagnostic(
      sessionId: string,
      signal: AbortSignal,
    ): Promise<DiagnosticProgress>
    readDiagnosticResult(
      sessionId: string,
      signal: AbortSignal,
    ): Promise<DiagnosticResult>
    readDiagnosticFeedback(
      sessionId: string,
      signal: AbortSignal,
    ): Promise<DiagnosticOverallFeedback>
    submitDiagnostic(
      input: DiagnosticSubmission,
      signal: AbortSignal,
    ): Promise<DiagnosticProgress>
    createSubmission(
      sessionId: string,
      taskId: string,
      blob: Blob,
      role: DiagnosticTask['role'],
    ): DiagnosticSubmission
    readDiagnosticAudio(
      sessionId: string,
      taskId: string,
      signal: AbortSignal,
    ): Promise<DiagnosticAudioMetadata>
    regenerateDiagnosticAudio(
      sessionId: string,
      taskId: string,
      signal: AbortSignal,
    ): Promise<DiagnosticAudioMetadata>
    fetchDiagnosticAudioFile(
      audioUrl: string,
      signal: AbortSignal,
    ): Promise<Blob>
  }
}
const Context = createContext<DiagnosticDependencies | null>(null)
export function DiagnosticDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: DiagnosticDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useDiagnosticDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Diagnostic dependencies provider is missing')
  return value
}
