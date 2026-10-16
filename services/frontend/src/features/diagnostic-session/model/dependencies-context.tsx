import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { DiagnosticApi } from '@/entities/diagnostic-session'

export type DiagnosticDependencies = {
  diagnostic: DiagnosticApi & {
    transcribeRecording(
      audio: Blob,
      signal: AbortSignal,
    ): Promise<{ id: string; text: string }>
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
