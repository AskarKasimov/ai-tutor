import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  CompetencyMapImport,
  CompetencyMapSummary,
  CourseMaterial,
} from '@/entities/competency-map'

export type CompetencyMapDependencies = {
  competencyMap: {
    read(subjectId: string, signal: AbortSignal): Promise<CompetencyMapSummary>
    import(
      subjectId: string,
      file: File,
      signal: AbortSignal,
    ): Promise<CompetencyMapImport>
    readSubjectMaterials(
      subjectId: string,
      signal?: AbortSignal,
    ): Promise<CourseMaterial[]>
    uploadCourseMaterials(
      subjectId: string,
      files: File[],
      signal?: AbortSignal,
    ): Promise<CourseMaterial[]>
    deleteCourseMaterial(
      subjectId: string,
      materialId: string,
      signal?: AbortSignal,
    ): Promise<void>
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
