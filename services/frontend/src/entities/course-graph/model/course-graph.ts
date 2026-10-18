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
  // built: from the course materials; fixture: the default graph Geek
  // examines by until the first build.
  source?: 'built' | 'fixture'
}

// Native graph payload supplied by the course-graph service.
export type CourseGraphView = {
  topics: {
    id: string
    title: string
    question?: string
    // The topic's theory assembled from the materials (Markdown).
    text?: string
    provides?: string[]
    requires?: string[]
    parent_title?: string | null
    sources?: TopicSource[]
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

export type TopicSource = {
  material_id: string
  block_id: number
  start: number
  end: number
  text: string
  page?: number | null
  week?: number | null
  kind?: string | null
  filename?: string | null
  kind_label?: string | null
  material_revision?: string | null
}
