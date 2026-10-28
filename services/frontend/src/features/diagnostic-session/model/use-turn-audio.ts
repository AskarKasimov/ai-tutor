import { useCallback, useEffect, useRef, useState } from 'react'
import type { Turn, TurnAudioPart } from '@/entities/diagnostic-session'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

export type TurnAudioState = {
  turnId: string | null
  status: 'idle' | 'loading' | 'playing' | 'error'
}

// Plays the voice of Geek turns, one at a time; fetched WAVs are kept for
// replays while the dialogue is open. A cached intro can play while the first
// reply block is synthesized; each block prefetches only its immediate successor.
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
    async (
      turn: Pick<Turn, 'id' | 'reply' | 'audioIntro' | 'replyAudioParts'>,
      onStart?: () => void,
    ) => {
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
        const count = turn.replyAudioParts ?? 0
        const indices: Array<number | undefined> =
          count > 0
            ? Array.from({ length: count }, (_, i) => i)
            : turn.reply.trim()
              ? [undefined]
              : []
        // Only one block ahead: stopping/answering does not enqueue all speech.
        let reply = indices.length ? load('reply', indices[0]) : null
        reply?.catch(() => undefined)
        const playAudio = async (blob: Blob) => {
          signal.throwIfAborted()
          await playToEnd(blob, signal, () => {
            start()
            if (mine()) setState({ turnId: turn.id, status: 'playing' })
          })
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
        }
        if (turn.audioIntro) {
          // A missing intro must never hide or prevent the actual feedback.
          const intro = await load('intro').catch(() => {
            signal.throwIfAborted()
            return null
          })
          if (intro) await playAudio(intro)
        }
        let question: Promise<Blob> | null = null
        for (let i = 0; i < indices.length; i++) {
          const audio = await reply!
          signal.throwIfAborted()
          reply = i + 1 < indices.length ? load('reply', indices[i + 1]) : null
          reply?.catch(() => undefined)
          if (i === indices.length - 1) {
            question = load('question')
            question.catch(() => undefined)
          }
          await playAudio(audio)
        }
        await playAudio(await (question ?? load('question')))
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
