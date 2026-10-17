export type CourseMaterialKind = string

export type CourseMaterialKindOption = {
  code: CourseMaterialKind
  label: string
}

export type CourseMaterial = {
  id: string
  subjectId: string
  filename: string
  contentType: string
  fileSize: number
  s3RawKey: string
  s3ParsedKey: string
  parsedText: string
  charCount: number
  status: 'pending' | 'parsed' | 'failed'
  week: number | null
  kind: CourseMaterialKind | null
  errorMessage?: string | null
  processing?: MaterialProcessing
  cleaning?: MaterialCleaning
  ocrWarnings: string[]
  createdAt: number
  updatedAt: number
}

export type MaterialProcessing = {
  stage:
    | 'queued'
    | 'waiting_models'
    | 'ocr'
    | 'translation'
    | 'cleaning'
    | 'graph'
    | 'ready'
    | 'failed'
  status: 'pending' | 'processing' | 'ready' | 'failed'
  updatedAt: number
  attempts: number
}

export type MaterialCleaning = {
  status: 'pending' | 'processing' | 'ready' | 'failed'
  updatedAt: number
  attempts: number
  errorMessage: string
}

export type CourseGraphTopic = {
  id: string
  title: string
  question?: string
  provides?: string[]
  requires?: string[]
  parent_title?: string | null
}

export type CourseGraphPrerequisite = {
  source: string
  target: string
  requirement?: string
  reason?: string
}

export type CourseGraphHierarchyEdge = {
  source: string
  target: string
}

export type CourseGraph = {
  topics: CourseGraphTopic[]
  prerequisites: CourseGraphPrerequisite[]
  hierarchy: CourseGraphHierarchyEdge[]
  quality?: {
    acyclic: boolean
    topics?: number
    requirements?: number
    unresolved_requirements?: number
  }
}

export type CourseGraphSnapshot = {
  status: 'not_requested' | 'pending' | 'processing' | 'ready' | 'failed'
  attempts: number
  errorMessage: string
  graph: CourseGraph | null
  stale: boolean
  // Every material is processed, so a build can start.
  inputsReady: boolean
  updatedAt: number
}
