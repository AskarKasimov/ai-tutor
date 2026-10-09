export const subjectQueryKeys = {
  subjects: (userId: string) => ['subjects', userId] as const,
  learningState: (userId: string, subjectId: string) =>
    ['subjects', userId, subjectId, 'learning-state'] as const,
  createSubject: (userId: string) => ['subjects', userId, 'create'] as const,
}
