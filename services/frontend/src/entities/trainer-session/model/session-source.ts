import type { SessionAnswer, TrainerSession } from '@/entities/trainer-session'

// Local demo contract. Real diagnostics use the server-owned flow in
// use-diagnostic-session; demo answers never enter the diagnostic API.
export interface TrainerSessionSource {
  getSession(userId: string): Promise<TrainerSession>
  saveAnswer(
    userId: string,
    sessionId: string,
    answer: SessionAnswer,
  ): Promise<TrainerSession>
  restart(userId: string): Promise<TrainerSession>
  clear(userId: string): void
}
