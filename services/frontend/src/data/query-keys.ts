export const queryKeys = {
  auth: ['auth', 'me'] as const,
  competencyMap: (userId: string) => ['competency-map', userId] as const,
  trainerSession: (userId: string | undefined) =>
    ['trainer-session', userId] as const,
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
  authenticate: ['auth', 'authenticate'] as const,
  logout: ['auth', 'logout'] as const,
  importCompetencyMap: (userId: string) =>
    ['competency-map', 'import', userId] as const,
  startDiagnostic: (userId: string) => ['diagnostic', 'start', userId] as const,
  submitDiagnostic: (userId: string, sessionId: string) =>
    ['diagnostic', 'submit', userId, sessionId] as const,
  transcribe: (userId: string) => ['voice', 'transcribe', userId] as const,
  synthesize: (userId: string) => ['voice', 'synthesize', userId] as const,
  evaluate: (userId: string) => ['assessment', 'evaluate', userId] as const,
}
