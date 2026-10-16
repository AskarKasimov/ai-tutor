import type { DiagnosticAnswer } from '@/entities/diagnostic-session'

// The idempotency key of an answer: the same reply to the same turn repeats
// the request, so a retry after a failure returns the accepted result; another
// reply is a new request.
export function answerKey(turnId: string, answer: DiagnosticAnswer) {
  const value =
    answer.kind === 'text'
      ? answer.text
      : answer.kind === 'voice'
        ? answer.transcriptionId
        : ''
  let hash = 5381
  for (let i = 0; i < value.length; i++)
    hash = ((hash << 5) + hash + value.charCodeAt(i)) | 0
  return `answer:${turnId}:${answer.kind}:${(hash >>> 0).toString(36)}:${value.length}`
}
