export class CompetencyMapApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly details: { path?: string; message: string }[] = [],
  ) {
    super(message)
    this.name = 'CompetencyMapApiError'
  }
}
