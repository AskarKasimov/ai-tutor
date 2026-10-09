export class SubjectApiError extends Error {
  constructor(
    readonly status: number,
    readonly code = '',
  ) {
    super(`Subject request failed: ${status}`)
    this.name = 'SubjectApiError'
  }
}
