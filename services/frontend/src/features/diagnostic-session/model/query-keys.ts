export const diagnosticSessionQueryKeys = {
  history: (userId: string, subjectId: string) =>
    ['diagnostic-history', userId, subjectId] as const,
  session: (userId: string, sessionId: string) =>
    ['diagnostic-session', userId, sessionId] as const,
  result: (userId: string, sessionId: string) =>
    ['diagnostic-result', userId, sessionId] as const,
  start: (userId: string, subjectId: string) =>
    ['diagnostic', 'start', userId, subjectId] as const,
  answer: (userId: string, sessionId: string) =>
    ['diagnostic', 'answer', userId, sessionId] as const,
}
