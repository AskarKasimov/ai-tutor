import { useCallback, useEffect, useRef, useState } from 'react'
import type { Turn, TurnAudioPart } from '@/entities/diagnostic-session'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

const FEEDBACK_PAUSE_MS = 700

export type TurnAudioState = {
  turnId: string | null
  status: 'idle' | 'loading' | 'playing' | 'error' | 'paused'
}

type Progress = {
  intro: boolean
  replies: number
  question: boolean
  revealed: boolean
  transition?: boolean
  interrupted?: boolean
}

// Plays the voice of Geek turns, one at a time; fetched WAVs are kept for
// replays while the dialogue is open. The cached intro can play while the
// complete feedback is synthesized; the question is prefetched afterwards.
export function useTurnAudio(sessionId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const [state, setState] = useState<TurnAudioState>({
    turnId: null,
    status: 'idle',
  })
  const cache = useRef(new Map<string, Blob>())
  const current = useRef<AbortController | null>(null)
  const activeTurn = useRef<string | null>(null)
  const [progress, setProgress] = useState<Record<string, Progress>>({})

  const revealAll = useCallback((turnId: string) => {
    current.current?.abort()
    current.current = null
    activeTurn.current = null
    setState({ turnId: null, status: 'idle' })
    setProgress((old) => ({
      ...old,
      [turnId]: { intro: false, replies: 0, question: false, revealed: true },
    }))
  }, [])

  const stop = useCallback(() => {
    current.current?.abort()
    current.current = null
    activeTurn.current = null
    setState((previous) =>
      previous.turnId
        ? { ...previous, status: 'paused' }
        : { turnId: null, status: 'idle' },
    )
  }, [])

  // onStart runs once, when the first part starts to play or when the turn
  // cannot be voiced (a failure, a blocked autoplay, a stop), so a caller
  // waiting for the voice is never left waiting.
  const play = useCallback(
    async (
      turn: Pick<
        Turn,
        | 'id'
        | 'kind'
        | 'reply'
        | 'audioIntro'
        | 'audioTransition'
        | 'replyAudioParts'
        | 'text'
      >,
      onStart?: () => void,
    ) => {
      const interruptedId = activeTurn.current
      if (interruptedId && interruptedId !== turn.id) {
        setProgress((old) => ({
          ...old,
          [interruptedId]: {
            ...(old[interruptedId] ?? {
              intro: false,
              replies: 0,
              question: false,
              revealed: false,
            }),
            interrupted: true,
          },
        }))
      }
      current.current?.abort()
      const controller = new AbortController()
      current.current = controller
      activeTurn.current = turn.id
      setProgress((old) => ({
        ...old,
        [turn.id]: {
          ...(old[turn.id] ?? {
            intro: false,
            replies: 0,
            question: false,
            revealed: false,
          }),
          interrupted: false,
        },
      }))
      const { signal } = controller
      const mine = () => current.current === controller
      let started = false
      const start = () => {
        if (started) return
        started = true
        onStart?.()
      }
      setState({ turnId: turn.id, status: 'loading' })

      const load = async (part: TurnAudioPart, chunk?: number) => {
        const key = `${turn.id}:${part}:${chunk ?? 'whole'}`
        let audio = cache.current.get(key)
        if (!audio) {
          audio = await diagnostic.fetchTurnAudio(
            sessionId,
            turn.id,
            signal,
            part,
            chunk,
          )
          cache.current.set(key, audio)
        }
        return audio
      }

      try {
        const hasReply = Boolean(turn.reply.trim())
        const reply = hasReply ? load('reply') : null
        reply?.catch(() => undefined)
        const playAudio = async (
          blob: Blob,
          part: 'intro' | 'reply' | 'transition' | 'question',
          index = 0,
        ) => {
          signal.throwIfAborted()
          await playToEnd(blob, signal, () => {
            start()
            if (mine())
              setProgress((old) => {
                const prior = old[turn.id] ?? {
                  intro: false,
                  replies: 0,
                  question: false,
                  revealed: false,
                }
                return {
                  ...old,
                  [turn.id]: {
                    ...prior,
                    intro: prior.intro || part === 'intro',
                    replies:
                      part === 'reply'
                        ? Math.max(prior.replies, index + 1)
                        : prior.replies,
                    question: prior.question || part === 'question',
                    transition: prior.transition || part === 'transition',
                  },
                }
              })
            if (mine()) setState({ turnId: turn.id, status: 'playing' })
          })
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
        }
        if (turn.audioIntro) {
          await playAudio(await load('intro'), 'intro')
        }
        const questionText =
          turn.reply && turn.text.trimStart().startsWith(turn.reply)
            ? turn.text.trimStart().slice(turn.reply.length).trim()
            : turn.text.trim()
        let question: Promise<Blob> | null = null
        if (reply) {
          const audio = await reply!
          signal.throwIfAborted()
          if (questionText) {
            question = load('question')
            question.catch(() => undefined)
          }
          await playAudio(audio, 'reply')
        }
        if (hasReply && turn.kind !== 'closing') {
          // Keep the question prefetched, but let the feedback sink in.
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
          await pauseAfterFeedback(signal)
        }
        if (turn.audioTransition)
          await playAudio(await load('transition'), 'transition')
        if (questionText)
          await playAudio(await (question ?? load('question')), 'question')
        else if (mine())
          setProgress((old) => ({
            ...old,
            [turn.id]: {
              ...(old[turn.id] ?? {
                intro: false,
                replies: 0,
                revealed: false,
              }),
              question: true,
            },
          }))
        if (mine()) {
          current.current = null
          activeTurn.current = null
          setState({ turnId: null, status: 'idle' })
        }
      } catch {
        start()
        if (signal.aborted) return
        if (mine())
          setState({
            turnId: turn.id,
            status: 'error',
          })
      }
    },
    [diagnostic, sessionId],
  )

  useEffect(() => () => current.current?.abort(), [])
  return { state, play, stop, progress, revealAll }
}

// Plays one WAV and settles when it ends, fails or is aborted.
function playToEnd(audio: Blob, signal: AbortSignal, onStart: () => void) {
  return new Promise<void>((resolve, reject) => {
    const abort = () => reject(signal.reason)
    signal.addEventListener('abort', abort, { once: true })
    const settle = (fn: () => void) => () => {
      signal.removeEventListener('abort', abort)
      fn()
    }
    playQuestion(
      audio,
      settle(resolve),
      (error) => settle(() => reject(error))(),
      signal,
    ).then(onStart, (error: unknown) => settle(() => reject(error))())
  })
}

function pauseAfterFeedback(signal: AbortSignal) {
  signal.throwIfAborted()
  return new Promise<void>((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer)
      reject(signal.reason)
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, FEEDBACK_PAUSE_MS)
    signal.addEventListener('abort', abort, { once: true })
  })
}
