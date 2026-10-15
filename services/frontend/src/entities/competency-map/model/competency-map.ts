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
