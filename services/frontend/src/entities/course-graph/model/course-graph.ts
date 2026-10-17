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
  stale?: boolean
  inputsReady?: boolean
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
  stale?: boolean
  inputsReady?: boolean
  view?: CourseGraphView
}

// Native graph payload supplied by the course-graph service.
export type CourseGraphView = {
  topics: {
    id: string
    title: string
    question?: string
    provides?: string[]
    requires?: string[]
    parent_title?: string | null
  }[]
  prerequisites: {
    source: string
    target: string
    requirement?: string
    reason?: string
  }[]
  hierarchy: { source: string; target: string }[]
  quality?: {
    acyclic: boolean
    topics?: number
    requirements?: number
    unresolved_requirements?: number
    [key: string]: unknown
  }
}
