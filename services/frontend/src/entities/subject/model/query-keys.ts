export const subjectQueryKeys = {
  subjects: (userId: string) => ['subjects', userId] as const,
  createSubject: (userId: string) => ['subjects', userId, 'create'] as const,
  audioPreparation: (userId: string, subjectId: string) =>
    ['subjects', userId, subjectId, 'audio-preparation'] as const,
}
