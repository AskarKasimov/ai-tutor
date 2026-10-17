export type CourseGraphBuildStatus =
  'queued' | 'running' | 'succeeded' | 'failed'

export type CourseGraphBuildStage =
  'cleaning' | 'units' | 'dedup' | 'prerequisites' | 'validation'

export type CourseGraphBuildError = {
  code: string
  message: string
}

export type CourseGraphBuild = {
  id: string
  subjectId: string
  status: CourseGraphBuildStatus
  stage: CourseGraphBuildStage | null
  startedAt: number | null
  completedAt: number | null
  error: CourseGraphBuildError | null
}

export type CourseGraphNode = {
  id: string
  title: string
  content: string
  prerequisites: string[]
  outcomes: string[]
  questions: string[]
}

export type CourseGraph = {
  version: string
  publishedAt: number | null
  sourceMaterialIds?: string[]
  nodes: CourseGraphNode[]
}
