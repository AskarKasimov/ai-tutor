export class TrainingApiError extends Error {
  constructor(
    readonly status: number,
    readonly code = '',
  ) {
    super(`Training request failed: ${status}`)
    this.name = 'TrainingApiError'
  }
}
