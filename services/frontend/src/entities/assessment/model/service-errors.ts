export class AssessmentApiError extends Error {
  constructor(readonly status: number) {
    super(`Assessment failed: ${status}`)
    this.name = 'AssessmentApiError'
  }
}
