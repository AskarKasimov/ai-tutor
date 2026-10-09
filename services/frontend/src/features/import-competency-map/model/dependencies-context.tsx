import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  CompetencyMapImport,
  CompetencyMapSummary,
} from '@/entities/competency-map'
export type CompetencyMapDependencies = {
  competencyMap: {
    read(subjectId: string, signal: AbortSignal): Promise<CompetencyMapSummary>
    import(
      subjectId: string,
      file: File,
      signal: AbortSignal,
    ): Promise<CompetencyMapImport>
  }
}
const Context = createContext<CompetencyMapDependencies | null>(null)
export function CompetencyMapDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: CompetencyMapDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useCompetencyMapDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Competency map dependencies provider is missing')
  return value
}
