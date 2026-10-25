export const subjectQueryKeys = {
  subjects: (userId: string) => ['subjects', userId, 'learn'] as const,
  learningState: (userId: string, subjectId: string) =>
    ['subjects', userId, 'learn', subjectId, 'learning-state'] as const,
  createSubject: (userId: string) =>
    ['subjects', userId, 'learn', 'create'] as const,
}
