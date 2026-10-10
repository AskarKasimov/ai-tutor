// Client-side record of answered question states for the session navigator.
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

// A diagnostic answer may skip the following basic positions, so the answered
// index is derived from the new totals and the newly skipped count.
export function recordDiagnosticAnswer(
  track: QuestionTrack,
  response: {
    completed_tasks: number
    skipped_tasks: number
    score?: number
    grader_max_score?: number
  },
): { track: QuestionTrack; index: number } {
  const done = response.completed_tasks + response.skipped_tasks
  const newlySkipped = Math.max(0, response.skipped_tasks - track.skipped)
  const index = Math.max(0, done - 1 - newlySkipped)
  const states = padded(track.states, index)
  states.push(scoreState(response.score ?? 0, response.grader_max_score ?? 1))
  for (let i = 0; i < newlySkipped; i++) states.push('skipped')
  return { track: { states, skipped: response.skipped_tasks }, index }
}

// Training answers carry their own 1-based sequence number.
export function recordTrainingAnswer(
  track: QuestionTrack,
  sequence: number,
  score: number,
  maxScore: number,
): QuestionTrack {
  const states = padded(track.states, Math.max(track.states.length, sequence))
  states[sequence - 1] = scoreState(score, maxScore)
  return { states, skipped: 0 }
}

export function initialTrack(done: number, skipped: number): QuestionTrack {
  return { states: padded([], done), skipped }
}
