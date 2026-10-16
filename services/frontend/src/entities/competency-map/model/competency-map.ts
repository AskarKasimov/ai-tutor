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
