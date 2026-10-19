// A detail of a rejected request: for a competency map, path "file" names a
// cell of the map with its problem.
export type CourseGraphApiErrorDetail = {
  path?: string
  code: string
  message: string
}

export class CourseGraphApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details: CourseGraphApiErrorDetail[] = [],
  ) {
    super(message)
    this.name = 'CourseGraphApiError'
  }
}
