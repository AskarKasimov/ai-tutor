export const competencyMapQueryKeys = {
  competencyMap: (userId: string, subjectId: string) =>
    ['competency-map', userId, subjectId] as const,
  importCompetencyMap: (userId: string, subjectId: string) =>
    ['competency-map', 'import', userId, subjectId] as const,
}
