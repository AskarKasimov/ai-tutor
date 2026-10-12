export const learningPlanQueryKeys = {
  byDiagnostic: (userId: string, diagnosticId: string) =>
    ['learning-plan', userId, 'diagnostic', diagnosticId] as const,
  messages: (userId: string, planId: string, index: number) =>
    ['learning-plan', userId, planId, 'messages', index] as const,
}
