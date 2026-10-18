import { useCallback, useEffect, useRef, useState } from 'react'
import type { Turn, TurnAudioPart } from '@/entities/diagnostic-session'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

export type TurnAudioState = {
  turnId: string | null
  status: 'idle' | 'loading' | 'playing' | 'error'
}

// Plays the voice of Geek turns, one at a time; fetched WAVs are kept for
// replays while the dialogue is open. A turn is voiced in two parts: the short
// reply to the previous answer plays first, while the question after it is
// still being synthesized. The question is requested only once the reply has
// arrived, because the speech service works through requests one by one.
export function useTurnAudio(sessionId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const [state, setState] = useState<TurnAudioState>({
    turnId: null,
    status: 'idle',
  })
  const cache = useRef(new Map<string, Blob>())
  const current = useRef<AbortController | null>(null)

  const stop = useCallback(() => {
    current.current?.abort()
    current.current = null
    setState({ turnId: null, status: 'idle' })
  }, [])

  // onStart runs once, when the first part starts to play or when the turn
  // cannot be voiced (a failure, a blocked autoplay, a stop), so a caller
  // waiting for the voice is never left waiting.
  const play = useCallback(
    async (turn: Pick<Turn, 'id' | 'reply'>, onStart?: () => void) => {
      current.current?.abort()
      const controller = new AbortController()
      current.current = controller
      const { signal } = controller
      const mine = () => current.current === controller
      let started = false
      const start = () => {
        if (started) return
        started = true
        onStart?.()
      }
      setState({ turnId: turn.id, status: 'loading' })

      const load = async (part: TurnAudioPart) => {
        const key = `${turn.id}:${part}`
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
        const reply = turn.reply.trim() ? await load('reply') : null
        const question = load('question')
        // The question is awaited after the reply has played; until then its
        // failure must not surface as an unhandled rejection.
        question.catch(() => undefined)
        if (reply) {
          signal.throwIfAborted()
          await playToEnd(reply, signal, () => {
            start()
            if (mine()) setState({ turnId: turn.id, status: 'playing' })
          })
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
        }
        const audio = await question
        signal.throwIfAborted()
        await playToEnd(audio, signal, () => {
          start()
          if (mine()) setState({ turnId: turn.id, status: 'playing' })
        })
        if (mine()) {
          current.current = null
          setState({ turnId: null, status: 'idle' })
        }
      } catch (error) {
        start()
        if (signal.aborted) return
        // A browser that blocks autoplay is not an error: the student can
        // press play.
        const blocked =
          error instanceof DOMException && error.name === 'NotAllowedError'
        if (mine())
          setState({
            turnId: blocked ? null : turn.id,
            status: blocked ? 'idle' : 'error',
          })
      }
    },
    [diagnostic, sessionId],
  )

  useEffect(() => () => current.current?.abort(), [])
  return { state, play, stop }
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
