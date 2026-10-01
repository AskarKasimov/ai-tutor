import { useEffect, useRef, useState } from 'react'
import { createAudioUrl, playQuestion, startRecording } from '../platform/prototype-audio'
import { synthesizeQuestion, transcribeRecording, VoiceApiError } from './voice-api'
import type { Recording } from '../platform/prototype-audio'

type Stage = 'ready' | 'permission' | 'recording' | 'processing' | 'result' | 'error'

export function useTrainerPrototype(question: string) {
  const [stage, setStage] = useState<Stage>('ready')
  const [seconds, setSeconds] = useState(0)
  const [error, setError] = useState('')
  const [speaking, setSpeaking] = useState(false)
  const [speechError, setSpeechError] = useState('')
  const [transcript, setTranscript] = useState('')
  const [example, setExample] = useState(false)
  const [loadingSpeech, setLoadingSpeech] = useState(false)
  const speechRequest = useRef<AbortController | null>(null)
  const transcriptionRequest = useRef<AbortController | null>(null)
  const [audioUrl, setAudioUrl] = useState<string>()
  const recording = useRef<Recording | null>(null)
  const cancelSpeech = useRef<(() => void) | null>(null)
  const savedAudio = useRef<ReturnType<typeof createAudioUrl> | null>(null)
  const processingTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const busy = useRef(false)
  const generation = useRef(0)
  const startedAt = useRef(0)

  useEffect(() => () => {
    generation.current++
    recording.current?.dispose()
    speechRequest.current?.abort()
    transcriptionRequest.current?.abort()
    cancelSpeech.current?.()
    savedAudio.current?.dispose()
    if (processingTimer.current) clearTimeout(processingTimer.current)
  }, [])

  useEffect(() => {
    if (stage !== 'recording') return
    const timer = setInterval(() => setSeconds(Math.floor((Date.now() - startedAt.current) / 1000)), 250)
    return () => clearInterval(timer)
  }, [stage])

  function stopSpeaking() {
    speechRequest.current?.abort()
    speechRequest.current = null
    setLoadingSpeech(false)
    cancelSpeech.current?.()
    cancelSpeech.current = null
    setSpeaking(false)
  }

  async function speak() {
    if (speaking || loadingSpeech) { stopSpeaking(); return }
    const request = new AbortController()
    speechRequest.current = request
    setSpeechError('')
    setLoadingSpeech(true)
    try {
      const blob = await synthesizeQuestion(question, request.signal)
      if (request.signal.aborted) return
      const dispose = await playQuestion(blob, () => setSpeaking(false), () => {
        setSpeaking(false)
        setSpeechError('speechError')
      })
      if (request.signal.aborted) { dispose(); return }
      cancelSpeech.current = dispose
      setSpeaking(true)
    } catch (error) {
      if (!request.signal.aborted) setSpeechError(error instanceof VoiceApiError && error.status === 401 ? 'speechUnauthorized' : 'speechError')
    } finally {
      if (speechRequest.current === request) setLoadingSpeech(false)
    }
  }

  async function start() {
    if (busy.current) return
    busy.current = true
    transcriptionRequest.current = null
    const current = ++generation.current
    stopSpeaking()
    setError('')
    setTranscript('')
    setExample(false)
    setStage('permission')
    try {
      const capture = await startRecording()
      if (generation.current !== current) { capture.dispose(); return }
      recording.current = capture
      startedAt.current = Date.now()
      setSeconds(0)
      setStage('recording')
    } catch (error) {
      if (generation.current !== current) return
      setError(error instanceof Error && error.name === 'NotAllowedError' ? 'denied' : 'unavailable')
      setStage('error')
    } finally {
      if (generation.current === current) busy.current = false
    }
  }

  function showExample() {
    stopSpeaking()
    setExample(true)
    setTranscript('')
    setStage('processing')
    processingTimer.current = setTimeout(() => {
      setStage('result')
      processingTimer.current = null
    }, 1300)
  }

  async function stop() {
    if (busy.current || !recording.current) return
    busy.current = true
    setStage('processing')
    const current = generation.current
    try {
      const blob = await recording.current.stop()
      if (generation.current !== current) return
      recording.current = null
      if (!blob.size) throw new Error('empty')
      savedAudio.current?.dispose()
      savedAudio.current = createAudioUrl(blob)
      setAudioUrl(savedAudio.current.url)
      const request = new AbortController()
      transcriptionRequest.current = request
      const text = await transcribeRecording(blob, request.signal)
      if (generation.current !== current) return
      setTranscript(text)
      setExample(false)
      setStage('result')
    } catch (error) {
      if (generation.current !== current) return
      setError(error instanceof VoiceApiError && error.status === 401 ? 'unauthorized' : transcriptionRequest.current ? 'transcription' : 'capture')
      setStage('error')
    } finally {
      if (generation.current === current) busy.current = false
    }
  }

  function reset() {
    generation.current++
    transcriptionRequest.current?.abort()
    transcriptionRequest.current = null
    stopSpeaking()
    recording.current?.dispose()
    recording.current = null
    savedAudio.current?.dispose()
    savedAudio.current = null
    setAudioUrl(undefined)
    setTranscript('')
    setExample(false)
    setError('')
    setSpeechError('')
    setSeconds(0)
    setStage('ready')
    busy.current = false
    if (processingTimer.current) clearTimeout(processingTimer.current)
  }

  return { stage, seconds, error, speaking, loadingSpeech, speechError, audioUrl, transcript, example, speak, start, stop, showExample, reset }
}
