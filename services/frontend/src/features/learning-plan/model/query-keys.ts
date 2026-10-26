export const learningPlanQueryKeys = {
  byDiagnostic: (userId: string, diagnosticId: string) =>
    ['learn', 'learning-plan', userId, 'diagnostic', diagnosticId] as const,
  messages: (userId: string, planId: string, index: number) =>
    ['learn', 'learning-plan', userId, planId, 'messages', index] as const,
}
