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
  processing?: MaterialProcessing
  cleaning?: MaterialCleaning
  ocrWarnings?: string[]
  errorMessage?: string | null
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
