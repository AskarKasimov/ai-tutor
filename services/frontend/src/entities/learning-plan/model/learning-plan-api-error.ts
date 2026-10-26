export class LearningPlanApiError extends Error {
  constructor(
    readonly status: number,
    readonly code = '',
  ) {
    super(`Learning plan request failed: ${status}`)
    this.name = 'LearningPlanApiError'
  }
}
