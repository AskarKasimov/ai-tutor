export const diagnosticSessionQueryKeys = {
  diagnosticProgress: (userId: string, sessionId: string | undefined) =>
    ['diagnostic-session', userId, sessionId] as const,
  diagnosticResults: (userId: string) => ['diagnostic-result', userId] as const,
  diagnosticResult: (userId: string, sessionId: string) =>
    ['diagnostic-result', userId, sessionId] as const,
  diagnosticAudio: (
    userId: string,
    sessionId: string,
    taskId: string | undefined,
  ) => ['diagnostic-audio', userId, sessionId, taskId] as const,
  startDiagnostic: (userId: string) => ['diagnostic', 'start', userId] as const,
  submitDiagnostic: (userId: string, sessionId: string) =>
    ['diagnostic', 'submit', userId, sessionId] as const,
}
