export const diagnosticSessionQueryKeys = {
  diagnosticProgress: (userId: string, sessionId: string | undefined) =>
    ['learn', 'diagnostic-session', userId, sessionId] as const,
  diagnosticResults: (userId: string) =>
    ['learn', 'diagnostic-result', userId] as const,
  diagnosticResult: (userId: string, sessionId: string) =>
    ['learn', 'diagnostic-result', userId, sessionId] as const,
  diagnosticFeedback: (userId: string, sessionId: string) =>
    ['learn', 'diagnostic-feedback', userId, sessionId] as const,
  diagnosticAudio: (
    userId: string,
    sessionId: string,
    taskId: string | undefined,
  ) => ['learn', 'diagnostic-audio', userId, sessionId, taskId] as const,
  startDiagnostic: (userId: string) =>
    ['learn', 'diagnostic', 'start', userId] as const,
  submitDiagnostic: (userId: string, sessionId: string) =>
    ['learn', 'diagnostic', 'submit', userId, sessionId] as const,
}
