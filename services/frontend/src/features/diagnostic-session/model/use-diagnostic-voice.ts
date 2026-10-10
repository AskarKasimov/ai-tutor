import { useEffect, useRef, useState } from 'react'
import {
  AudioPlaybackError,
  createAudioUrl,
  playQuestion,
  speakInstruction,
  startRecording,
} from '@/shared/lib'
import { StoredAudioError } from '@/shared/api'
import type { Recording } from '@/shared/lib'
import type {
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import type { DiagnosticSubmission } from '@/entities/diagnostic-session'
import { useDiagnosticDependencies } from './dependencies-context'
import { useDiagnosticAudioQuery } from './use-voice-operations'
import { useMutation, useQueryClient } from '@tanstack/react-query'
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
  const repair = useMutation({
    mutationKey: ['diagnostic-audio-regenerate', userId, sessionId],
    retry: false,
    gcTime: 0,
    mutationFn: ({ taskId, signal }: { taskId: string; signal: AbortSignal }) =>
      diagnostic.regenerateDiagnosticAudio(sessionId, taskId, signal),
  })
  const recording = useRef<Recording | null>(null)
  const pending = useRef<{
    input: DiagnosticSubmission
    task: DiagnosticTask
  } | null>(null)
  const audio = useRef<ReturnType<typeof createAudioUrl> | null>(null)
  const answerRequest = useRef<AbortController | null>(null)
  const speechRequest = useRef<AbortController | null>(null)
  const player = useRef<(() => void) | null>(null)
  const playerGeneration = useRef(0)
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
  // auto: played on task open; never stops current playback and stays silent
  // when the browser blocks autoplay, leaving the manual button.
  async function speak({ auto = false }: { auto?: boolean } = {}) {
    if (speechRequest.current) {
      if (auto && !speechRequest.current.signal.aborted) return
      if (!auto) {
        stopSpeech()
        return
      }
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
    let repairAttempted = false
    let repairPromise: Promise<void> | null = null
    const stillCurrent = () =>
      !request.signal.aborted &&
      generation.current === currentGeneration &&
      currentContext.current.taskId === taskId &&
      currentContext.current.sessionId === sessionId &&
      currentContext.current.userId === userId
    // When stored audio is not ready or unplayable, the browser voice reads
    // the instruction so a task never stays silent.
    const instruction = task?.voice_instruction ?? ''
    const startFallback = () => {
      if (!stillCurrent()) return false
      const stop = speakInstruction(instruction, () => {
        if (!stillCurrent() || player.current !== stop) return
        speechRequest.current = null
        player.current = null
        setSpeech('idle')
      })
      if (!stop) return false
      player.current = stop
      setSpeechError(false)
      setPendingHint(false)
      setCancelledHint(false)
      setSpeech('playing')
      return true
    }
    const audioKey = (audioUrl: string) =>
      diagnosticSessionQueryKeys.storedTaskAudio(
        userId,
        sessionId,
        taskId,
        audioUrl,
      )
    const canRepair = (error: unknown) =>
      error instanceof StoredAudioError &&
      (error.kind === 'invalid' ||
        error.kind === 'network' ||
        error.status === 404 ||
        error.status >= 500)
    let metadata:
      Awaited<ReturnType<typeof diagnostic.readDiagnosticAudio>> | undefined
    let repairBadMetadata = false
    try {
      try {
        const metadataResult = await audioQuery.refetch({ throwOnError: true })
        metadata = metadataResult.data
      } catch (error) {
        repairBadMetadata =
          error instanceof DiagnosticApiError &&
          ((error.status === 503 &&
            error.code === 'AUDIO_STORAGE_UNAVAILABLE') ||
            (error.status === 0 && error.code === 'INVALID_RESPONSE'))
        if (!repairBadMetadata) throw error
      }
      if (!stillCurrent()) return
      if (!repairBadMetadata && !metadata)
        throw new Error('Diagnostic audio metadata is empty')
      if (
        metadata &&
        (metadata.status === 'pending' || metadata.status === 'processing')
      ) {
        if (startFallback()) return
        speechRequest.current = null
        setSpeech('idle')
        setPendingHint(true)
        return
      }
      if (metadata?.status === 'cancelled') {
        if (startFallback()) return
        speechRequest.current = null
        setSpeech('idle')
        setCancelledHint(true)
        return
      }
      if (metadata?.status === 'failed') repairBadMetadata = true
      if (
        !repairBadMetadata &&
        (metadata?.status !== 'ready' || !metadata.audio_url)
      ) {
        if (startFallback()) return
        speechRequest.current = null
        setSpeech('idle')
        setSpeechError(true)
        return
      }
      const fetchBlob = (url: string) =>
        cache.fetchQuery({
          queryKey: audioKey(url),
          queryFn: ({ signal }) =>
            diagnostic.fetchDiagnosticAudioFile(url, signal),
          staleTime: Infinity,
        })
      const playBlob = async (
        blob: Blob,
        url: string,
        isRepairReplay = false,
      ): Promise<void> => {
        if (!stillCurrent()) return
        const thisPlayer = ++playerGeneration.current
        let finished = false
        const finish = () => {
          finished = true
          if (!stillCurrent() || thisPlayer !== playerGeneration.current) return
          speechRequest.current = null
          player.current = null
          setSpeech('idle')
        }
        const recover = (reason: unknown) => {
          if (
            !(reason instanceof AudioPlaybackError) ||
            (reason.kind !== 'decode' && reason.kind !== 'source') ||
            !stillCurrent()
          )
            return Promise.resolve()
          if (repairPromise) return repairPromise
          if (isRepairReplay || repairAttempted) {
            const error = new Error(
              'Stored audio is still unreadable after repair',
            )
            onError(error)
            if (startFallback()) return Promise.resolve()
            setSpeechError(true)
            setSpeech('idle')
            speechRequest.current = null
            return Promise.resolve()
          }
          repairAttempted = true
          setSpeech('loading')
          repairPromise = (async () => {
            cache.removeQueries({ queryKey: audioKey(url), exact: true })
            const repaired = await repair.mutateAsync({
              taskId,
              signal: request.signal,
            })
            if (!stillCurrent()) return
            if (repaired.status !== 'ready' || !repaired.audio_url)
              throw new Error('Audio repair did not return ready metadata')
            const repairedURL = repaired.audio_url
            cache.removeQueries({
              queryKey: audioKey(repairedURL),
              exact: true,
            })
            const repairedBlob = await fetchBlob(repairedURL)
            if (!stillCurrent()) return
            await playBlob(repairedBlob, repairedURL, true)
          })()
          const currentRepair = repairPromise
          void currentRepair.then(
            () => {
              if (repairPromise === currentRepair) repairPromise = null
            },
            (error: unknown) => {
              if (repairPromise === currentRepair) repairPromise = null
              if (!stillCurrent()) return
              onError(error)
              if (startFallback()) return
              setSpeechError(true)
              setSpeech('idle')
              speechRequest.current = null
            },
          )
          return repairPromise
        }
        try {
          const dispose = await playQuestion(blob, finish, (error) => {
            if (thisPlayer !== playerGeneration.current || !stillCurrent())
              return
            finished = true
            player.current = null
            void recover(error)
          })
          if (!stillCurrent() || thisPlayer !== playerGeneration.current) {
            dispose()
            return
          }
          if (!finished) {
            player.current = dispose
            setSpeech('playing')
          }
        } catch (error) {
          if (thisPlayer !== playerGeneration.current || !stillCurrent()) return
          if (
            error instanceof DOMException &&
            error.name === 'NotAllowedError'
          ) {
            finish()
            throw error
          }
          if (error instanceof AudioPlaybackError) {
            await recover(error)
            return
          }
          throw error
        }
      }
      if (repairBadMetadata) {
        repairAttempted = true
        const repaired = await repair.mutateAsync({
          taskId,
          signal: request.signal,
        })
        if (!stillCurrent()) return
        if (repaired.status !== 'ready' || !repaired.audio_url)
          throw new Error('Audio repair did not return ready metadata')
        const repairedBlob = await fetchBlob(repaired.audio_url)
        if (!stillCurrent()) return
        await playBlob(repairedBlob, repaired.audio_url, true)
        return
      }
      let blob: Blob
      try {
        blob = await fetchBlob(metadata!.audio_url!)
      } catch (error) {
        if (!canRepair(error) || repairAttempted) throw error
        repairAttempted = true
        cache.removeQueries({
          queryKey: audioKey(metadata!.audio_url!),
          exact: true,
        })
        const repaired = await repair.mutateAsync({
          taskId,
          signal: request.signal,
        })
        if (!stillCurrent()) return
        if (repaired.status !== 'ready' || !repaired.audio_url)
          throw new Error('Audio repair did not return ready metadata')
        blob = await fetchBlob(repaired.audio_url)
        if (!stillCurrent()) return
        await playBlob(blob, repaired.audio_url, true)
        return
      }
      if (!stillCurrent()) return
      await playBlob(blob, metadata!.audio_url!)
    } catch (error) {
      if (!request.signal.aborted) {
        const blocked =
          error instanceof DOMException && error.name === 'NotAllowedError'
        if (!(error instanceof DiagnosticApiError && error.status === 409))
          if (!blocked) onError(error)
        // Blocked autoplay keeps the manual button; other failures fall back.
        if (!(auto && blocked) && startFallback()) return
        speechRequest.current = null
        if (
          !(error instanceof DiagnosticApiError && error.status === 409) &&
          !(auto && blocked) &&
          !auto
        )
          setSpeechError(true)
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
  // Sends prepared audio, such as the spoken skip phrase, as the answer.
  async function submitAudio(blob: Blob) {
    if (locked.current || recording.current || !task) return
    locked.current = true
    const current = generation.current
    const capturedTask = task
    stopSpeech()
    setError(undefined)
    setCaptureError('')
    try {
      audio.current?.dispose()
      audio.current = null
      setAudioUrl(undefined)
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
    submitAudio,
    speak,
    reset,
  }
}
