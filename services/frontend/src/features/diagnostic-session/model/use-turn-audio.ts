import { useCallback, useEffect, useRef, useState } from 'react'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import type { Turn, TurnAudioPart } from '@/entities/diagnostic-session'
import { playPcmStream, playQuestion, supportsPcmStreaming } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

export type TurnAudioState = {
  turnId: string | null
  status: 'idle' | 'loading' | 'playing' | 'error'
}

type ActivePlayback = { controller: AbortController; release: () => void }
type TurnAudio = Blob | ReadableStream<Uint8Array>

// A reply is received before requesting the question. The question can then
// synthesize while the bounded reply queue finishes playing.
export function useTurnAudio(sessionId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const [state, setState] = useState<TurnAudioState>({
    turnId: null,
    status: 'idle',
  })
  const cache = useRef(new Map<string, Blob>())
  const current = useRef<ActivePlayback | null>(null)
  const stop = useCallback(() => {
    current.current?.release()
    current.current?.controller.abort()
    current.current = null
    setState({ turnId: null, status: 'idle' })
  }, [])

  const play = useCallback(
    async (turn: Pick<Turn, 'id' | 'reply'>, onStart?: () => void) => {
      current.current?.release()
      current.current?.controller.abort()
      const controller = new AbortController()
      const { signal } = controller
      let started = false
      const start = () => {
        if (!started) {
          started = true
          onStart?.()
        }
      }
      const active = { controller, release: start }
      current.current = active
      const mine = () => current.current === active
      setState({ turnId: turn.id, status: 'loading' })
      const streams = new Set<ReadableStream<Uint8Array>>()
      const key = (part: TurnAudioPart) => `${sessionId}:${turn.id}:${part}`
      const load = async (part: TurnAudioPart): Promise<TurnAudio> => {
        signal.throwIfAborted()
        const cached = cache.current.get(key(part))
        if (cached) return cached
        if (diagnostic.fetchTurnAudioStream && supportsPcmStreaming()) {
          try {
            const stream = await diagnostic.fetchTurnAudioStream(
              sessionId,
              turn.id,
              signal,
              part,
            )
            if (signal.aborted) {
              await stream.cancel().catch(() => undefined)
              signal.throwIfAborted()
            }
            streams.add(stream)
            return stream
          } catch (error) {
            signal.throwIfAborted()
            if (
              !(error instanceof DiagnosticApiError) ||
              ![404, 406, 501].includes(error.status)
            )
              throw error
          }
        }
        const audio = await diagnostic.fetchTurnAudio(
          sessionId,
          turn.id,
          signal,
          part,
        )
        signal.throwIfAborted()
        cache.current.set(key(part), audio)
        return audio
      }
      const voice = async (
        audio: TurnAudio,
        part: TurnAudioPart,
        received?: () => void,
      ) => {
        signal.throwIfAborted()
        const playing = () => {
          start()
          if (mine()) setState({ turnId: turn.id, status: 'playing' })
        }
        if (audio instanceof Blob) {
          received?.()
          await playToEnd(audio, signal, playing)
        } else {
          const result = await playPcmStream(audio, signal, playing, received)
          signal.throwIfAborted()
          cache.current.set(key(part), result.audio)
        }
      }
      let question: Promise<TurnAudio> | undefined
      const requestQuestion = () => {
        question ??= load('question')
        // Its error belongs to the question's playback, after the reply.
        void question.catch(() => undefined)
      }
      try {
        if (turn.reply.trim()) {
          await voice(await load('reply'), 'reply', requestQuestion)
          if (mine()) setState({ turnId: turn.id, status: 'loading' })
        }
        requestQuestion()
        await voice(await question!, 'question')
        if (mine()) setState({ turnId: null, status: 'idle' })
      } catch (error) {
        start()
        if (signal.aborted) return
        const blocked =
          error instanceof DOMException && error.name === 'NotAllowedError'
        if (mine())
          setState({
            turnId: blocked ? null : turn.id,
            status: blocked ? 'idle' : 'error',
          })
      } finally {
        controller.abort()
        for (const stream of streams)
          if (!stream.locked) void stream.cancel().catch(() => undefined)
        if (mine()) current.current = null
      }
    },
    [diagnostic, sessionId],
  )

  useEffect(() => {
    setState({ turnId: null, status: 'idle' })
    const entries = cache.current
    return () => {
      current.current?.release()
      current.current?.controller.abort()
      current.current = null
      entries.clear()
    }
  }, [sessionId])
  return { state, play, stop }
}

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
