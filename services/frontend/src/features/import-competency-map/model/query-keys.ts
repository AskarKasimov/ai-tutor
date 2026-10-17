export const competencyMapQueryKeys = {
  competencyMap: (userId: string, subjectId: string) =>
    ['competency-map', userId, subjectId] as const,
  importCompetencyMap: (userId: string, subjectId: string) =>
    ['competency-map', 'import', userId, subjectId] as const,
  subjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', userId, subjectId] as const,
  uploadSubjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', 'upload', userId, subjectId] as const,
  courseGraph: (userId: string, subjectId: string) =>
    ['course-graph', userId, subjectId] as const,
}
