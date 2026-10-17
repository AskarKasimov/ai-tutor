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

// Where the course-graph service found a topic in the course materials.
export type CourseGraphSource = {
  material_id: string
  page?: number | null
  week?: number | null
}

export type CourseGraphTopic = {
  id: string
  title: string
  question?: string
  // The topic's theory assembled from the materials (Markdown).
  text?: string
  provides?: string[]
  requires?: string[]
  sources?: CourseGraphSource[]
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
  // built: from the course materials; fixture: the default graph Geek
  // examines by until the first build.
  graphSource: 'built' | 'fixture' | 'none'
  stale: boolean
  // Every material is processed, so a build can start.
  inputsReady: boolean
  updatedAt: number
}
