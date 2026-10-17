export type CompetencyMapSummary = {
  revision: number
  importedAt: number | null
  competencyCount: number
  constituentCount: number
  outcomeCount: number
  taskCount: number
}

export type CompetencyMapImport = CompetencyMapSummary & {
  unparsedTaskCellCount: number
  warnings: { row: number; columnIndex: number; column: string; code: string }[]
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
  errorMessage?: string | null
  createdAt: number
  updatedAt: number
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
  status: 'waiting_materials' | 'pending' | 'processing' | 'ready' | 'failed'
  attempts: number
  errorMessage: string
  graph: CourseGraph | null
  stale: boolean
  updatedAt: number
}
