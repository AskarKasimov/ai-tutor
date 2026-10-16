import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type {
  CourseGraphSnapshot,
  CourseMaterial,
  CourseMaterialUploadResult,
  CourseMaterialMetadataPatch,
  CourseMaterialKind,
} from '@/entities/competency-map'

export type CompetencyMapDependencies = {
  competencyMap: {
    readSubjectMaterials(
      subjectId: string,
      signal?: AbortSignal,
    ): Promise<CourseMaterial[]>
    uploadCourseMaterials(
      subjectId: string,
      files: File[],
      week: number,
      kind: CourseMaterialKind,
      signal?: AbortSignal,
    ): Promise<CourseMaterialUploadResult>
    updateCourseMaterialMetadata(
      subjectId: string,
      materialId: string,
      patch: CourseMaterialMetadataPatch,
      signal?: AbortSignal,
    ): Promise<CourseMaterial>
    deleteCourseMaterial(
      subjectId: string,
      materialId: string,
      signal?: AbortSignal,
    ): Promise<void>
    readCourseGraph(
      subjectId: string,
      signal?: AbortSignal,
    ): Promise<CourseGraphSnapshot>
    requestCourseGraphBuild(
      subjectId: string,
      signal?: AbortSignal,
    ): Promise<CourseGraphSnapshot>
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
