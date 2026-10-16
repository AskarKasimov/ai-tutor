import { useCallback, useEffect, useRef, useState } from 'react'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

export type TurnAudioState = {
  turnId: string | null
  status: 'idle' | 'loading' | 'playing' | 'error'
}

// Plays the voice of Geek turns, one at a time; fetched WAVs are kept for
// replays while the dialogue is open.
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

  const play = useCallback(
    async (turnId: string) => {
      current.current?.abort()
      const controller = new AbortController()
      current.current = controller
      setState({ turnId, status: 'loading' })
      try {
        let audio = cache.current.get(turnId)
        if (!audio) {
          audio = await diagnostic.fetchTurnAudio(
            sessionId,
            turnId,
            controller.signal,
          )
          cache.current.set(turnId, audio)
        }
        controller.signal.throwIfAborted()
        await playQuestion(
          audio,
          () => {
            if (current.current === controller) {
              current.current = null
              setState({ turnId: null, status: 'idle' })
            }
          },
          () => {
            if (current.current === controller)
              setState({ turnId, status: 'error' })
          },
          controller.signal,
        )
        if (current.current === controller)
          setState({ turnId, status: 'playing' })
      } catch (error) {
        if (controller.signal.aborted) return
        // A browser that blocks autoplay is not an error: the student can
        // press play.
        const blocked =
          error instanceof DOMException && error.name === 'NotAllowedError'
        setState({
          turnId: blocked ? null : turnId,
          status: blocked ? 'idle' : 'error',
        })
      }
    },
    [diagnostic, sessionId],
  )

  useEffect(() => () => current.current?.abort(), [])
  return { state, play, stop }
}
