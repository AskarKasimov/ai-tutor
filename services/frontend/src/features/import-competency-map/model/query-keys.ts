export const competencyMapQueryKeys = {
  subjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', userId, subjectId] as const,
  uploadSubjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', 'upload', userId, subjectId] as const,
  courseGraph: (userId: string, subjectId: string) =>
    ['course-graph', userId, subjectId] as const,
}
