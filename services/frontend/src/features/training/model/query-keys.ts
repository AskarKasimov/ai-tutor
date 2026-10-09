export const trainingQueryKeys = {
  entry: (userId: string | undefined, diagnosticId: string | undefined) =>
    ['training-entry', userId, diagnosticId] as const,
  session: (userId: string | undefined, sessionId: string | undefined) =>
    ['training-session', userId, sessionId] as const,
  history: (userId: string | undefined, sessionId: string | undefined) =>
    ['training-history', userId, sessionId] as const,
  audio: (
    userId: string | undefined,
    sessionId: string | undefined,
    exerciseId: string | undefined,
  ) => ['training-audio', userId, sessionId, exerciseId] as const,
}
