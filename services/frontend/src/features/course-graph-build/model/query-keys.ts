export const courseGraphQueryKeys = {
  build: (userId: string, subjectId: string) =>
    ['course-graph-build', userId, subjectId] as const,
  graph: (userId: string, subjectId: string) =>
    ['course-graph', userId, subjectId] as const,
}
