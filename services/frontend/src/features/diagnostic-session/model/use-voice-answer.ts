import { useCallback, useEffect, useRef, useState } from 'react'
import { VoiceApiError } from '@/shared/api'
import { useDiagnosticDependencies } from './dependencies-context'
import { useVoiceRecorder } from './use-voice-recorder'

export type VoiceAnswerError = 'no_speech' | 'transcription'

// A spoken answer: the recording is transcribed and shown to the student, who
// sends it or records again. identity resets everything for a new Geek turn.
export function useVoiceAnswer(identity: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const [transcript, setTranscript] = useState<{
    id: string
    text: string
  } | null>(null)
  const [transcribing, setTranscribing] = useState(false)
  const [error, setError] = useState<VoiceAnswerError | null>(null)
  const request = useRef<AbortController | null>(null)
  const lastAudio = useRef<Blob | null>(null)

  const recognize = useCallback(
    async (audio: Blob) => {
      request.current?.abort()
      const controller = new AbortController()
      request.current = controller
      lastAudio.current = audio
      setTranscript(null)
      setError(null)
      setTranscribing(true)
      try {
        const result = await diagnostic.transcribeRecording(
          audio,
          controller.signal,
        )
        if (!controller.signal.aborted) setTranscript(result)
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof VoiceApiError &&
              cause.code === 'NO_SPEECH_DETECTED'
              ? 'no_speech'
              : 'transcription',
          )
      } finally {
        if (request.current === controller) {
          request.current = null
          setTranscribing(false)
        }
      }
    },
    [diagnostic],
  )
  const recorder = useVoiceRecorder(identity, (audio) => void recognize(audio))

  const discard = useCallback(() => {
    request.current?.abort()
    request.current = null
    lastAudio.current = null
    setTranscript(null)
    setTranscribing(false)
    setError(null)
  }, [])
  useEffect(() => discard, [identity, discard])

  return {
    ...recorder,
    transcript,
    transcribing,
    transcriptionError: error,
    discard,
    retryTranscription: () => {
      if (lastAudio.current) void recognize(lastAudio.current)
    },
  }
}
