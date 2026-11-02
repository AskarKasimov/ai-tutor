import { useCallback, useEffect, useRef, useState } from 'react'
import type { Turn, TurnAudioPart } from '@/entities/diagnostic-session'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

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
// replays while the dialogue is open. Feedback and question play in order.
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

      const load = async (part?: TurnAudioPart) => {
        const key = `${turn.id}:${part ?? 'whole'}`
        let audio = cache.current.get(key)
        if (!audio) {
          audio = await diagnostic.fetchTurnAudio(
            sessionId,
            turn.id,
            signal,
            part,
          )
          cache.current.set(key, audio)
        }
        return audio
      }

      try {
        const text = turn.text.trimStart()
        const split = Boolean(
          turn.reply &&
          text.startsWith(turn.reply) &&
          text.slice(turn.reply.length).trim(),
        )
        const playPart = async (
          audio: Blob,
          part: 'reply' | 'question' | 'whole',
        ) => {
          signal.throwIfAborted()
          await playToEnd(audio, signal, () => {
            start()
            if (mine()) {
              setProgress((old) => ({
                ...old,
                [turn.id]: {
                  ...(old[turn.id] ?? {
                    intro: false,
                    replies: 0,
                    question: false,
                    revealed: false,
                  }),
                  replies: part === 'reply' ? 1 : (old[turn.id]?.replies ?? 0),
                  question: part !== 'reply' || Boolean(old[turn.id]?.question),
                  revealed: part === 'whole' || Boolean(old[turn.id]?.revealed),
                },
              }))
              setState({ turnId: turn.id, status: 'playing' })
            }
          })
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
        }
        if (split) {
          const reply = load('reply')
          const question = load('question')
          question.catch(() => undefined)
          await playPart(await reply, 'reply')
          await playPart(await question, 'question')
        } else {
          await playPart(await load(), 'whole')
        }
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
