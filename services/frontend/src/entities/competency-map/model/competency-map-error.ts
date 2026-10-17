export class CompetencyMapApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly details: { path?: string; message: string }[] = [],
    readonly code = '',
  ) {
    super(message)
    this.name = 'CompetencyMapApiError'
  }
}
