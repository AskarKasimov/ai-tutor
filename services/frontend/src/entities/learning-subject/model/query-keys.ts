export const subjectQueryKeys = {
  subjects: (userId: string) => ['learn', 'subjects', userId] as const,
  learningState: (userId: string, subjectId: string) =>
    ['learn', 'subjects', userId, subjectId, 'learning-state'] as const,
  createSubject: (userId: string) =>
    ['learn', 'subjects', userId, 'create'] as const,
}
