export const trainerSessionQueryKeys = {
  session: (userId: string | undefined) => ['trainer-session', userId] as const,
}
