// Client-side record of answered question states for the training navigator.
// The server reveals each score only in its answer response, so the states are
// kept per session in sessionStorage and survive a page reload.

export type TrackState =
  'correct' | 'partial' | 'incorrect' | 'skipped' | 'unknown'

export type QuestionTrack = { states: TrackState[]; skipped: number }

const storageKey = (sessionId: string) => `ai-tutor:question-track:${sessionId}`

export function readTrack(sessionId: string): QuestionTrack | null {
  try {
    const value = JSON.parse(
      sessionStorage.getItem(storageKey(sessionId)) ?? 'null',
    ) as QuestionTrack | null
    return value &&
      Array.isArray(value.states) &&
      typeof value.skipped === 'number'
      ? value
      : null
  } catch {
    return null
  }
}

export function writeTrack(sessionId: string, track: QuestionTrack) {
  try {
    sessionStorage.setItem(storageKey(sessionId), JSON.stringify(track))
  } catch {
    /* The navigator falls back to neutral colors without storage. */
  }
}

export function scoreState(score: number, maxScore: number): TrackState {
  if (score >= maxScore) return 'correct'
  return score <= 0 ? 'incorrect' : 'partial'
}

function padded(states: TrackState[], length: number) {
  const next = states.slice(0, length)
  while (next.length < length) next.push('unknown')
  return next
}

// Training answers carry their own 1-based sequence number.
export function recordTrainingAnswer(
  track: QuestionTrack,
  sequence: number,
  score: number,
  maxScore: number,
  skipped = false,
): QuestionTrack {
  const states = padded(track.states, Math.max(track.states.length, sequence))
  states[sequence - 1] = skipped ? 'skipped' : scoreState(score, maxScore)
  return { states, skipped: 0 }
}

export function initialTrack(done: number, skipped: number): QuestionTrack {
  return { states: padded([], done), skipped }
}
