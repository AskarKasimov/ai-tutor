import { useCallback, useEffect, useRef, useState } from 'react'
import { MAX_ANSWER_SECONDS, prepareSpeech } from '@/shared/lib'

type RecordingError = 'denied' | 'unavailable' | null

// Records one spoken answer; identity resets the recorder for a new question.
export function useVoiceRecorder(
  identity: string,
  onRecording: (audio: Blob) => void,
) {
  const callback = useRef(onRecording)
  const recorder = useRef<MediaRecorder | null>(null)
  const stream = useRef<MediaStream | null>(null)
  const active = useRef(false)
  const generation = useRef(0)
  const startedAt = useRef(0)
  const timer = useRef<ReturnType<typeof setInterval> | null>(null)
  const [recording, setRecording] = useState(false)
  const [requesting, setRequesting] = useState(false)
  const [streamState, setStreamState] = useState<MediaStream>()
  const [elapsedSeconds, setElapsedSeconds] = useState(0)
  const [error, setError] = useState<RecordingError>(null)
  callback.current = onRecording

  const stop = useCallback(() => {
    if (recorder.current?.state === 'recording') recorder.current.stop()
  }, [])

  const start = useCallback(async () => {
    if (recording || requesting || !active.current) return
    const currentGeneration = generation.current
    if (
      !navigator.mediaDevices?.getUserMedia ||
      typeof MediaRecorder === 'undefined'
    ) {
      setError('unavailable')
      return
    }
    setError(null)
    setRequesting(true)
    let media: MediaStream | undefined
    try {
      media = await navigator.mediaDevices.getUserMedia({ audio: true })
      if (!active.current || generation.current !== currentGeneration) {
        media.getTracks().forEach((track) => track.stop())
        return
      }
      const mimeType = [
        'audio/webm;codecs=opus',
        'audio/ogg;codecs=opus',
        'audio/wav',
      ].find((type) => MediaRecorder.isTypeSupported(type))
      const instance = new MediaRecorder(
        media,
        mimeType ? { mimeType } : undefined,
      )
      const chunks: BlobPart[] = []
      stream.current = media
      recorder.current = instance
      setStreamState(media)
      startedAt.current = Date.now()
      setElapsedSeconds(0)
      if (timer.current) clearInterval(timer.current)
      timer.current = setInterval(() => {
        if (generation.current !== currentGeneration) return
        const elapsed = Math.floor((Date.now() - startedAt.current) / 1000)
        setElapsedSeconds(elapsed)
        if (elapsed >= MAX_ANSWER_SECONDS && instance.state === 'recording')
          instance.stop()
      }, 250)
      instance.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data)
      }
      instance.onstop = () => {
        media?.getTracks().forEach((track) => track.stop())
        if (generation.current !== currentGeneration) return
        if (timer.current) clearInterval(timer.current)
        timer.current = null
        if (stream.current === media) stream.current = null
        if (recorder.current === instance) recorder.current = null
        setStreamState(undefined)
        setRecording(false)
        const audio = new Blob(chunks, {
          type: instance.mimeType || 'audio/webm',
        })
        if (!audio.size) return
        void prepareSpeech(audio).then((prepared) => {
          if (generation.current === currentGeneration)
            callback.current(prepared)
        })
      }
      instance.start()
      setRecording(true)
    } catch (cause) {
      media?.getTracks().forEach((track) => track.stop())
      if (generation.current === currentGeneration) {
        if (timer.current) clearInterval(timer.current)
        timer.current = null
        if (stream.current === media) stream.current = null
        recorder.current = null
        setStreamState(undefined)
        setRecording(false)
        setError(
          cause instanceof DOMException && cause.name === 'NotAllowedError'
            ? 'denied'
            : 'unavailable',
        )
      }
    } finally {
      if (active.current && generation.current === currentGeneration)
        setRequesting(false)
    }
  }, [recording, requesting])

  useEffect(() => {
    generation.current += 1
    const currentGeneration = generation.current
    active.current = true
    setStreamState(undefined)
    setRecording(false)
    setRequesting(false)
    setElapsedSeconds(0)
    setError(null)
    return () => {
      active.current = false
      if (generation.current === currentGeneration) generation.current += 1
      if (timer.current) clearInterval(timer.current)
      timer.current = null
      const oldRecorder = recorder.current
      const oldStream = stream.current
      stream.current = null
      recorder.current = null
      oldStream?.getTracks().forEach((track) => track.stop())
      if (oldRecorder?.state === 'recording') oldRecorder.stop()
    }
  }, [identity])

  return {
    recording,
    requesting,
    stream: streamState,
    elapsedSeconds,
    error,
    start,
    stop,
  }
}
