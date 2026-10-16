import type { DiagnosticAnswer } from '@/entities/diagnostic-session'

// Idempotency keys of answers: the exact same reply to the same turn gets the
// same key, so a retry after a failure returns the accepted result; any other
// reply, however similar, gets a new key. Payloads are compared exactly, not
// hashed, so different replies never share a key.
export function createAnswerKeys(
  newId: () => string = () => crypto.randomUUID(),
) {
  const keys = new Map<string, string>()
  return (turnId: string, answer: DiagnosticAnswer) => {
    const value =
      answer.kind === 'text'
        ? answer.text
        : answer.kind === 'voice'
          ? answer.transcriptionId
          : ''
    const payload = JSON.stringify([turnId, answer.kind, value])
    let key = keys.get(payload)
    if (!key) {
      key = `answer:${newId()}`
      keys.set(payload, key)
    }
    return key
  }
}
