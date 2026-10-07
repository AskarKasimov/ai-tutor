import { useEffect, useRef, useState } from 'react'
import { createAudioUrl, playQuestion, startRecording } from '../platform/prototype-audio'
import type { Recording } from '../platform/prototype-audio'
import type { DiagnosticProgress, DiagnosticTask } from '../shared/domain'
import { createSubmission, diagnosticAudio } from './diagnostic-api'
import type { DiagnosticSubmission } from './diagnostic-api'

type Stage = 'ready' | 'permission' | 'recording' | 'processing' | 'error' | 'result'
export function useDiagnosticVoice(sessionId: string, task: DiagnosticTask | undefined,
  submit: (submission: DiagnosticSubmission, signal: AbortSignal) => Promise<DiagnosticProgress>,
  onError: (error: unknown) => void) {
  const [stage, setStage] = useState<Stage>('ready')
  const [error, setError] = useState<unknown>()
  const [captureError, setCaptureError] = useState('')
  const [seconds, setSeconds] = useState(0)
  const [stream, setStream] = useState<MediaStream>()
  const [audioUrl, setAudioUrl] = useState<string>()
  const [accepted, setAccepted] = useState<{ task: DiagnosticTask; progress: DiagnosticProgress }>()
  const [speech, setSpeech] = useState<'idle' | 'loading' | 'playing'>('idle')
  const [speechError, setSpeechError] = useState(false)
  const recording = useRef<Recording | null>(null)
  const pending = useRef<{ input: DiagnosticSubmission; task: DiagnosticTask } | null>(null)
  const audio = useRef<ReturnType<typeof createAudioUrl> | null>(null)
  const answerRequest = useRef<AbortController | null>(null)
  const speechRequest = useRef<AbortController | null>(null)
  const player = useRef<(() => void) | null>(null)
  const generation = useRef(0)
  const locked = useRef(false)
  const started = useRef(0)

  useEffect(() => () => {
    generation.current++
    answerRequest.current?.abort()
    speechRequest.current?.abort()
    recording.current?.dispose()
    player.current?.()
    audio.current?.dispose()
  }, [])
  useEffect(() => {
    if (stage !== 'recording') return
    const timer = setInterval(() => setSeconds(Math.floor((Date.now() - started.current) / 1000)), 250)
    return () => clearInterval(timer)
  }, [stage])
  function stopSpeech() {
    speechRequest.current?.abort()
    speechRequest.current = null
    player.current?.()
    player.current = null
    setSpeech('idle')
  }
  async function speak() {
    if (speechRequest.current) { stopSpeech(); return }
    const request = new AbortController()
    speechRequest.current = request
    setSpeechError(false)
    setSpeech('loading')
    try {
      const blob = await diagnosticAudio(sessionId, request.signal)
      if (request.signal.aborted) return
      let finished = false
      const finish = () => {
        finished = true
        if (request.signal.aborted) return
        speechRequest.current = null
        player.current = null
        setSpeech('idle')
      }
      const dispose = await playQuestion(blob, finish, () => { finish(); if (!request.signal.aborted) setSpeechError(true) })
      if (request.signal.aborted) { dispose(); return }
      if (!finished) { player.current = dispose; setSpeech('playing') }
    } catch (error) {
      if (!request.signal.aborted) { speechRequest.current = null; onError(error); setSpeechError(true); setSpeech('idle') }
    }
  }
  async function start() {
    if (locked.current || !task) return
    locked.current = true
    const current = ++generation.current
    stopSpeech()
    setError(undefined)
    setCaptureError('')
    setStage('permission')
    try {
      const capture = await startRecording()
      if (current !== generation.current) { capture.dispose(); return }
      audio.current?.dispose()
      audio.current = null
      setAudioUrl(undefined)
      recording.current = capture
      pending.current = null
      setStream(capture.stream)
      started.current = Date.now()
      setSeconds(0)
      setStage('recording')
    } catch (error) {
      if (current !== generation.current) return
      setCaptureError(error instanceof Error && error.name === 'NotAllowedError' ? 'denied' : 'unavailable')
      setStage('error')
    } finally { if (current === generation.current) locked.current = false }
  }
  async function send(current: number) {
    if (!pending.current) return
    const attempt = pending.current
    const request = new AbortController()
    answerRequest.current = request
    setStage('processing')
    setError(undefined)
    try {
      const progress = await submit(attempt.input, request.signal)
      if (current !== generation.current) return
      setAccepted({ task: attempt.task, progress })
      pending.current = null
      setStage('result')
    } catch (error) {
      if (current !== generation.current) return
      onError(error)
      setError(error)
      setStage('error')
    } finally { if (answerRequest.current === request) answerRequest.current = null }
  }
  async function stop() {
    if (locked.current || !recording.current || !task) return
    locked.current = true
    const current = generation.current
    const capturedTask = task
    setStage('processing')
    setStream(undefined)
    try {
      const blob = await recording.current.stop()
      recording.current = null
      if (current !== generation.current) return
      if (!blob.size) throw new Error('empty')
      audio.current = createAudioUrl(blob)
      setAudioUrl(audio.current.url)
      pending.current = { input: createSubmission(sessionId, capturedTask.variant_task_id, blob, capturedTask.role), task: capturedTask }
      await send(current)
    } catch {
      if (current !== generation.current) return
      setCaptureError('capture')
      setStage('error')
    } finally { if (current === generation.current) locked.current = false }
  }
  async function retry() {
    if (locked.current || !pending.current) return
    locked.current = true
    try { await send(generation.current) } finally { locked.current = false }
  }
  function reset() {
    generation.current++
    answerRequest.current?.abort()
    recording.current?.dispose()
    recording.current = null
    stopSpeech()
    audio.current?.dispose()
    audio.current = null
    pending.current = null
    locked.current = false
    setAudioUrl(undefined)
    setStream(undefined)
    setAccepted(undefined)
    setError(undefined)
    setCaptureError('')
    setSpeechError(false)
    setStage('ready')
  }
  return { stage, error, captureError, seconds, stream, audioUrl, accepted, speech, speechError, hasPending: !!pending.current, pendingTaskId: pending.current?.input.taskId, start, stop, retry, speak, reset }
}
