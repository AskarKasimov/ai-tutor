import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  CompetencyMapOutline,
  CompetencyMapImport,
  CompetencyMapSummary,
} from '@/entities/learning-competency-map'
export type CompetencyMapDependencies = {
  competencyMap: {
    readOutline(
      subjectId: string,
      signal: AbortSignal,
    ): Promise<CompetencyMapOutline>
    read(subjectId: string, signal: AbortSignal): Promise<CompetencyMapSummary>
    import(
      subjectId: string,
      file: File,
      expectedRevision: number,
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
