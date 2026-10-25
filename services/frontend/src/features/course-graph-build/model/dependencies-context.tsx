import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { CourseGraph, CourseGraphBuild } from '@/entities/course-graph'

export type CourseGraphBuildDependencies = {
  courseGraph: {
    readLatestCourseGraphBuild(
      subjectId: string,
      signal: AbortSignal,
    ): Promise<CourseGraphBuild | null>
    startCourseGraphBuild(
      subjectId: string,
      signal: AbortSignal,
    ): Promise<CourseGraphBuild>
    readCourseGraph(
      subjectId: string,
      signal: AbortSignal,
    ): Promise<CourseGraph>
  }
}

const Context = createContext<CourseGraphBuildDependencies | null>(null)

export function CourseGraphBuildDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: CourseGraphBuildDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}

export function useCourseGraphBuildDependencies() {
  const value = useContext(Context)
  if (!value)
    throw new Error('Course graph build dependencies provider is missing')
  return value
}
