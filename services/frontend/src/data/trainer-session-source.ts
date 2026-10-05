import type { SessionAnswer, TrainerSession } from '../shared/domain'

// Frontend contract. The future HTTP adapter must map Session/Assignment and
// resolve option/instruction data not yet exposed by the student API.
export interface TrainerSessionSource {
  getSession(userId: string): Promise<TrainerSession>
  saveAnswer(userId: string, sessionId: string, answer: SessionAnswer): Promise<TrainerSession>
  restart(userId: string): Promise<TrainerSession>
  clear(userId: string): void
}
