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
    answer_skipped?: boolean
  },
): { track: QuestionTrack; index: number } {
  const done = response.completed_tasks + response.skipped_tasks
  const newlySkipped = Math.max(0, response.skipped_tasks - track.skipped)
  const index = Math.max(0, done - 1 - newlySkipped)
  const states = padded(track.states, index)
  states.push(
    response.answer_skipped
      ? 'skipped'
      : scoreState(response.score ?? 0, response.grader_max_score ?? 1),
  )
  for (let i = 0; i < newlySkipped; i++) states.push('skipped')
  return { track: { states, skipped: response.skipped_tasks }, index }
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

// Competency (topic) progress: the number of topics is fixed while basics
// appear only when asked, so states are kept per topic and step
// (0 — main, 1–2 — basics).
export type TopicTrack = Record<number, TrackState[]>

const topicKey = (sessionId: string) => `ai-tutor:topic-track:${sessionId}`

export function readTopicTrack(sessionId: string): TopicTrack {
  try {
    const value = JSON.parse(
      sessionStorage.getItem(topicKey(sessionId)) ?? '{}',
    ) as TopicTrack
    return value && typeof value === 'object' ? value : {}
  } catch {
    return {}
  }
}

export function writeTopicTrack(sessionId: string, track: TopicTrack) {
  try {
    sessionStorage.setItem(topicKey(sessionId), JSON.stringify(track))
  } catch {
    /* The navigator falls back to neutral colors without storage. */
  }
}

export function recordTopicAnswer(
  track: TopicTrack,
  topic: number,
  step: number,
  state: TrackState,
): TopicTrack {
  const steps = [...(track[topic] ?? [])]
  while (steps.length < step) steps.push('unknown')
  steps[step] = state
  return { ...track, [topic]: steps }
}
