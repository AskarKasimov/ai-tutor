import { useEffect, useRef, useState } from 'react'
import { createAudioUrl, playQuestion, startRecording } from '@/shared/lib'
import type { Recording } from '@/shared/lib'
import type {
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import type { DiagnosticSubmission } from '@/entities/diagnostic-session'
import { useDiagnosticDependencies } from './dependencies-context'
import { useDiagnosticAudioQuery } from './use-voice-operations'
import { useQueryClient } from '@tanstack/react-query'
import { diagnosticSessionQueryKeys } from './query-keys'

type Stage =
  'ready' | 'permission' | 'recording' | 'processing' | 'error' | 'result'
export function useDiagnosticVoice(
  userId: string,
  sessionId: string,
  task: DiagnosticTask | undefined,
  submit: (
    submission: DiagnosticSubmission,
    signal: AbortSignal,
  ) => Promise<DiagnosticProgress>,
  onError: (error: unknown) => void,
) {
  const { diagnostic } = useDiagnosticDependencies()
  const [stage, setStage] = useState<Stage>('ready')
  const [error, setError] = useState<unknown>()
  const [captureError, setCaptureError] = useState('')
  const [seconds, setSeconds] = useState(0)
  const [stream, setStream] = useState<MediaStream>()
  const [audioUrl, setAudioUrl] = useState<string>()
  const [accepted, setAccepted] = useState<{
    task: DiagnosticTask
    progress: DiagnosticProgress
  }>()
  const [speech, setSpeech] = useState<'idle' | 'loading' | 'playing'>('idle')
  const [speechError, setSpeechError] = useState(false)
  const [pendingHint, setPendingHint] = useState(false)
  const [cancelledHint, setCancelledHint] = useState(false)
  const cache = useQueryClient()
  const audioQuery = useDiagnosticAudioQuery(
    userId,
    sessionId,
    task?.variant_task_id,
  )
  const recording = useRef<Recording | null>(null)
  const pending = useRef<{
    input: DiagnosticSubmission
    task: DiagnosticTask
  } | null>(null)
  const audio = useRef<ReturnType<typeof createAudioUrl> | null>(null)
  const answerRequest = useRef<AbortController | null>(null)
  const speechRequest = useRef<AbortController | null>(null)
  const player = useRef<(() => void) | null>(null)
  const generation = useRef(0)
  const locked = useRef(false)
  const started = useRef(0)
  const currentContext = useRef({
    userId,
    sessionId,
    taskId: task?.variant_task_id,
  })
  currentContext.current = { userId, sessionId, taskId: task?.variant_task_id }

  useEffect(
    () => () => {
      generation.current++
      answerRequest.current?.abort()
      speechRequest.current?.abort()
      recording.current?.dispose()
      player.current?.()
      audio.current?.dispose()
    },
    [],
  )
  useEffect(() => {
    if (stage !== 'recording') return
    const timer = setInterval(
      () => setSeconds(Math.floor((Date.now() - started.current) / 1000)),
      250,
    )
    return () => clearInterval(timer)
  }, [stage])
  function stopSpeech() {
    speechRequest.current?.abort()
    speechRequest.current = null
    void cache.cancelQueries({
      queryKey: diagnosticSessionQueryKeys.diagnosticAudio(
        userId,
        sessionId,
        task?.variant_task_id,
      ),
      exact: true,
    })
    cache.removeQueries({
      queryKey: diagnosticSessionQueryKeys.diagnosticAudio(
        userId,
        sessionId,
        task?.variant_task_id,
      ),
      exact: true,
    })
    void cache.cancelQueries({
      queryKey: ['stored-task-audio', userId, sessionId, task?.variant_task_id],
    })
    cache.removeQueries({
      queryKey: ['stored-task-audio', userId, sessionId, task?.variant_task_id],
    })
    player.current?.()
    player.current = null
    setSpeech('idle')
    setPendingHint(false)
    setCancelledHint(false)
  }
  async function speak() {
    if (speechRequest.current) {
      stopSpeech()
      return
    }
    const request = new AbortController()
    speechRequest.current = request
    const currentGeneration = generation.current
    const taskId = task?.variant_task_id
    if (!taskId) {
      speechRequest.current = null
      return
    }
    setSpeechError(false)
    setPendingHint(false)
    setCancelledHint(false)
    setSpeech('loading')
    try {
      const metadataResult = await audioQuery.refetch({ throwOnError: true })
      const metadata = metadataResult.data
      if (
        request.signal.aborted ||
        generation.current !== currentGeneration ||
        currentContext.current.taskId !== taskId ||
        currentContext.current.sessionId !== sessionId ||
        currentContext.current.userId !== userId
      )
        return
      if (!metadata) throw new Error('Diagnostic audio metadata is empty')
      if (metadata.status === 'pending' || metadata.status === 'processing') {
        speechRequest.current = null
        setSpeech('idle')
        setPendingHint(true)
        return
      }
      if (metadata.status === 'cancelled') {
        speechRequest.current = null
        setSpeech('idle')
        setCancelledHint(true)
        return
      }
      if (metadata.status !== 'ready' || !metadata.audio_url) {
        speechRequest.current = null
        setSpeech('idle')
        setSpeechError(true)
        return
      }
      const blob = await cache.fetchQuery({
        queryKey: diagnosticSessionQueryKeys.storedTaskAudio(
          userId,
          sessionId,
          taskId,
          metadata.audio_url,
        ),
        queryFn: ({ signal }) =>
          diagnostic.fetchDiagnosticAudioFile(metadata.audio_url!, signal),
        staleTime: Infinity,
      })
      if (
        request.signal.aborted ||
        generation.current !== currentGeneration ||
        currentContext.current.taskId !== taskId ||
        currentContext.current.sessionId !== sessionId ||
        currentContext.current.userId !== userId
      )
        return
      if (!blob) throw new Error('Stored audio is empty')
      let finished = false
      const finish = () => {
        finished = true
        if (
          request.signal.aborted ||
          generation.current !== currentGeneration ||
          currentContext.current.taskId !== taskId
        )
          return
        speechRequest.current = null
        player.current = null
        setSpeech('idle')
      }
      const dispose = await playQuestion(blob, finish, () => {
        finish()
        if (!request.signal.aborted) setSpeechError(true)
      })
      if (
        request.signal.aborted ||
        generation.current !== currentGeneration ||
        currentContext.current.taskId !== taskId
      ) {
        dispose()
        return
      }
      if (!finished) {
        player.current = dispose
        setSpeech('playing')
      }
    } catch (error) {
      if (!request.signal.aborted) {
        speechRequest.current = null
        if (!(error instanceof DiagnosticApiError && error.status === 409)) {
          onError(error)
          setSpeechError(true)
        }
        setSpeech('idle')
      }
    }
  }
  useEffect(() => {
    return () => {
      speechRequest.current?.abort()
      generation.current++
      void cache.cancelQueries({
        queryKey: diagnosticSessionQueryKeys.diagnosticAudio(
          userId,
          sessionId,
          task?.variant_task_id,
        ),
        exact: true,
      })
      cache.removeQueries({
        queryKey: diagnosticSessionQueryKeys.diagnosticAudio(
          userId,
          sessionId,
          task?.variant_task_id,
        ),
        exact: true,
      })
      void cache.cancelQueries({
        queryKey: [
          'stored-task-audio',
          userId,
          sessionId,
          task?.variant_task_id,
        ],
      })
      cache.removeQueries({
        queryKey: [
          'stored-task-audio',
          userId,
          sessionId,
          task?.variant_task_id,
        ],
      })
      player.current?.()
    }
  }, [cache, userId, sessionId, task?.variant_task_id])
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
      if (current !== generation.current) {
        capture.dispose()
        return
      }
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
      setCaptureError(
        error instanceof Error && error.name === 'NotAllowedError'
          ? 'denied'
          : 'unavailable',
      )
      setStage('error')
    } finally {
      if (current === generation.current) locked.current = false
    }
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
    } finally {
      if (answerRequest.current === request) answerRequest.current = null
    }
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
      pending.current = {
        input: diagnostic.createSubmission(
          sessionId,
          capturedTask.variant_task_id,
          blob,
          capturedTask.role,
        ),
        task: capturedTask,
      }
      await send(current)
    } catch {
      if (current !== generation.current) return
      setCaptureError('capture')
      setStage('error')
    } finally {
      if (current === generation.current) locked.current = false
    }
  }
  async function retry() {
    if (locked.current || !pending.current) return
    locked.current = true
    try {
      await send(generation.current)
    } finally {
      locked.current = false
    }
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
    setPendingHint(false)
    setCancelledHint(false)
    setStage('ready')
  }
  return {
    stage,
    error,
    captureError,
    seconds,
    stream,
    audioUrl,
    accepted,
    speech,
    speechError,
    pendingHint,
    cancelledHint,
    hasPending: !!pending.current,
    pendingTaskId: pending.current?.input.taskId,
    start,
    stop,
    retry,
    speak,
    reset,
  }
}
