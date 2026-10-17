export const competencyMapQueryKeys = {
  materialKinds: (userId: string) => ['material-kinds', userId] as const,
  subjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', userId, subjectId] as const,
  // Every mutation of a course's materials shares this prefix, so screens can
  // wait for all of them before acting on the material set.
  changeSubjectMaterials: (userId: string, subjectId: string) =>
    ['subject-materials', 'change', userId, subjectId] as const,
  uploadSubjectMaterials: (userId: string, subjectId: string) =>
    [
      ...competencyMapQueryKeys.changeSubjectMaterials(userId, subjectId),
      'upload',
    ] as const,
}
