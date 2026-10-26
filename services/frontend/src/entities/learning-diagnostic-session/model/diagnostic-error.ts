export class DiagnosticApiError extends Error {
  constructor(
    readonly status: number,
    readonly code = '',
  ) {
    super(`Diagnostic request failed: ${status}`)
    this.name = 'DiagnosticApiError'
  }
}
