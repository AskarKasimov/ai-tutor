export const competencyMapQueryKeys = {
  competencyMap: (userId: string) => ['competency-map', userId] as const,
  importCompetencyMap: (userId: string) =>
    ['competency-map', 'import', userId] as const,
}
