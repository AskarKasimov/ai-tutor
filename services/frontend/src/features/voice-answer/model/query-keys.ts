export const voiceAnswerQueryKeys = {
  transcribe: (userId: string) => ['voice', 'transcribe', userId] as const,
  evaluate: (userId: string) => ['assessment', 'evaluate', userId] as const,
  overallFeedback: (userId: string) =>
    ['assessment', 'overall-feedback', userId] as const,
}
