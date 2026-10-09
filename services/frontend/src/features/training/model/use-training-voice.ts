import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { TrainingProgress, TrainingSubmission } from '@/entities/training'
import { TrainingApiError } from '@/entities/training'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
  userQueryKeys,
} from '@/entities/user'
import { createAudioUrl, playQuestion, startRecording } from '@/shared/lib'
import type { Recording } from '@/shared/lib'
import { useTrainingDependencies } from './dependencies-context'
import { trainingQueryKeys } from './query-keys'

export type TrainingSubmit = (
  submission: TrainingSubmission,
  signal: AbortSignal,
) => Promise<TrainingProgress>
type Stage =
  'ready' | 'permission' | 'recording' | 'processing' | 'result' | 'error'

export function useTrainingVoice(
  userId: string,
  progress: TrainingProgress,
  submit: TrainingSubmit,
  onError: (error: unknown) => void,
) {
  const { training } = useTrainingDependencies()
  const cache = useQueryClient()
  const auth = useQuery<{ id: string } | null>({
    queryKey: userQueryKeys.auth,
    enabled: false,
  }).data
  const [stage, setStage] = useState<Stage>('ready')
  const [error, setError] = useState<unknown>()
  const [captureError, setCaptureError] = useState('')
  const [seconds, setSeconds] = useState(0)
  const [stream, setStream] = useState<MediaStream>()
  const [answerUrl, setAnswerUrl] = useState<string>()
  const [accepted, setAccepted] = useState<{
    exercise: TrainingProgress['current']
    progress: TrainingProgress
  }>()
  const acceptedRef = useRef<typeof accepted>(undefined)
  acceptedRef.current = accepted
  const [audioStatus, setAudioStatus] = useState<
    | 'idle'
    | 'loading'
    | 'ready'
    | 'pending'
    | 'processing'
    | 'missing'
    | 'failed'
    | 'cancelled'
  >('idle')
  const [speaking, setSpeaking] = useState(false)
  const [speechError, setSpeechError] = useState(false)
  const recording = useRef<Recording | null>(null)
  const answerAudio = useRef<ReturnType<typeof createAudioUrl> | null>(null)
  const pending = useRef<{
    submission: TrainingSubmission
    exercise: TrainingProgress['current']
  } | null>(null)
  const answerRequest = useRef<AbortController | null>(null)
  const speechRequest = useRef<AbortController | null>(null)
  const player = useRef<(() => void) | null>(null)
  const generation = useRef(0)
  const locked = useRef(false)
  const lockToken = useRef(0)
  const started = useRef(0)
  const exercise = progress.current
  const exerciseId = exercise.exercise_id
  const context = useRef({ userId, sessionId: progress.session_id, exerciseId })
  const identity = `${userId}:${progress.session_id}`
  const previousIdentity = useRef(identity)
  if (previousIdentity.current !== identity) {
    previousIdentity.current = identity
    acceptedRef.current = undefined
    pending.current = null
  }
  context.current = { userId, sessionId: progress.session_id, exerciseId }

  const disposeAnswer = useCallback(() => {
    answerAudio.current?.dispose()
    answerAudio.current = null
    setAnswerUrl(undefined)
  }, [])
  const stopSpeech = useCallback(() => {
    speechRequest.current?.abort()
    speechRequest.current = null
    player.current?.()
    player.current = null
    setSpeaking(false)
  }, [])

  useEffect(() => {
    const timer =
      stage === 'recording'
        ? setInterval(
            () => setSeconds(Math.floor((Date.now() - started.current) / 1000)),
            250,
          )
        : undefined
    return () => {
      if (timer) clearInterval(timer)
    }
  }, [stage])

  useEffect(() => {
    return () => {
      generation.current++
      answerRequest.current?.abort()
      speechRequest.current?.abort()
      recording.current?.dispose()
      player.current?.()
      const key = trainingQueryKeys.audio(
        userId,
        progress.session_id,
        exerciseId,
      )
      void cache.cancelQueries({ queryKey: key, exact: true })
      cache.removeQueries({ queryKey: key, exact: true })
    }
  }, [cache, userId, progress.session_id, exerciseId])

  useEffect(() => () => answerAudio.current?.dispose(), [])

  useEffect(() => {
    setAccepted(undefined)
    setStage('ready')
    setError(undefined)
    setCaptureError('')
    setStream(undefined)
    pending.current = null
    generation.current++
    lockToken.current++
    locked.current = false
    answerRequest.current?.abort()
    recording.current?.dispose()
    recording.current = null
    disposeAnswer()
    stopSpeech()
  }, [identity, disposeAnswer, stopSpeech])

  useEffect(() => {
    if (auth === undefined || auth?.id === userId) return
    generation.current++
    lockToken.current++
    locked.current = false
    answerRequest.current?.abort()
    speechRequest.current?.abort()
    recording.current?.dispose()
    recording.current = null
    player.current?.()
    player.current = null
    pending.current = null
    setAccepted(undefined)
    setStream(undefined)
    setStage('ready')
    disposeAnswer()
  }, [auth, userId, disposeAnswer])

  useEffect(() => {
    const holdingResult = !!acceptedRef.current
    if (pending.current?.exercise.exercise_id !== exerciseId)
      pending.current = null
    generation.current++
    lockToken.current++
    locked.current = false
    answerRequest.current?.abort()
    recording.current?.dispose()
    recording.current = null
    setStream(undefined)
    if (!holdingResult) setStage('ready')
    setError(undefined)
    setCaptureError('')
    if (!holdingResult) disposeAnswer()
    stopSpeech()
    setAudioStatus('idle')
    setSpeechError(false)
  }, [exerciseId, disposeAnswer, stopSpeech])

  const start = useCallback(async () => {
    if (locked.current || accepted || acceptedRef.current || pending.current)
      return
    const operation = ++lockToken.current
    locked.current = true
    const current = ++generation.current
    stopSpeech()
    setError(undefined)
    setCaptureError('')
    setStage('permission')
    try {
      const capture = await startRecording()
      if (
        current !== generation.current ||
        context.current.exerciseId !== exerciseId
      ) {
        capture.dispose()
        return
      }
      recording.current?.dispose()
      recording.current = capture
      setStream(capture.stream)
      started.current = Date.now()
      setSeconds(0)
      setStage('recording')
    } catch (failure) {
      if (current !== generation.current) return
      setCaptureError(
        failure instanceof Error && failure.name === 'NotAllowedError'
          ? 'denied'
          : 'unavailable',
      )
      setStage('error')
    } finally {
      if (operation === lockToken.current) locked.current = false
    }
  }, [accepted, exerciseId, stopSpeech])

  const send = useCallback(
    async (current: number) => {
      const attempt = pending.current
      if (!attempt) return
      const token = captureSession(cache)
      const controller = new AbortController()
      answerRequest.current?.abort()
      answerRequest.current = controller
      setStage('processing')
      setError(undefined)
      try {
        const result = await submit(attempt.submission, controller.signal)
        if (
          current !== generation.current ||
          context.current.exerciseId !== attempt.exercise.exercise_id
        )
          return
        setAccepted({ exercise: attempt.exercise, progress: result })
        pending.current = null
        setStage('result')
      } catch (failure) {
        if (current !== generation.current || controller.signal.aborted) return
        if (
          isCurrentSession(cache, token) &&
          cache.getQueryData<{ id: string } | null>(userQueryKeys.auth)?.id ===
            userId &&
          failure instanceof TrainingApiError &&
          failure.status === 401
        )
          void replaceSession(cache, null)
        onError(failure)
        setError(failure)
        setStage('error')
      } finally {
        if (answerRequest.current === controller) answerRequest.current = null
      }
    },
    [cache, onError, submit, userId],
  )

  const stop = useCallback(async () => {
    if (locked.current || !recording.current || accepted) return
    const operation = ++lockToken.current
    locked.current = true
    const current = generation.current
    const capturedExercise = exercise
    const capture = recording.current
    setStage('processing')
    setStream(undefined)
    try {
      const blob = await capture.stop()
      recording.current = null
      if (
        current !== generation.current ||
        context.current.exerciseId !== capturedExercise.exercise_id
      )
        return
      if (!blob.size) throw new Error('empty')
      answerAudio.current?.dispose()
      answerAudio.current = createAudioUrl(blob)
      setAnswerUrl(answerAudio.current.url)
      pending.current = {
        submission: training.createTrainingSubmission(
          progress.session_id,
          capturedExercise.exercise_id,
          blob,
        ),
        exercise: capturedExercise,
      }
      await send(current)
    } catch (failure) {
      if (current !== generation.current) return
      setCaptureError(
        failure instanceof Error && failure.message === 'empty'
          ? 'empty'
          : 'capture',
      )
      setError(failure)
      setStage('error')
    } finally {
      if (operation === lockToken.current) locked.current = false
    }
  }, [accepted, exercise, progress.session_id, send, training])

  const retry = useCallback(async () => {
    if (locked.current || !pending.current) return
    const operation = ++lockToken.current
    locked.current = true
    try {
      await send(generation.current)
    } finally {
      if (operation === lockToken.current) locked.current = false
    }
  }, [send])

  const speak = useCallback(async () => {
    if (acceptedRef.current) return
    if (speechRequest.current) {
      stopSpeech()
      return
    }
    const current = generation.current
    const controller = new AbortController()
    speechRequest.current = controller
    setSpeechError(false)
    setAudioStatus('loading')
    const token = captureSession(cache)
    const request = new AbortController()
    const unregister = registerSessionRequest(cache, token, request)
    const signal = AbortSignal.any([controller.signal, request.signal])
    const isCurrent = () =>
      !signal.aborted &&
      current === generation.current &&
      context.current.exerciseId === exerciseId &&
      isCurrentSession(cache, token)
    try {
      const metadata = await training.readTrainingAudio(
        progress.session_id,
        exerciseId,
        signal,
      )
      if (!isCurrent()) return
      setAudioStatus(metadata.status)
      if (metadata.status === 'failed') {
        setAudioStatus('loading')
        const repaired = await training.regenerateTrainingAudio(
          progress.session_id,
          exerciseId,
          signal,
        )
        if (!isCurrent()) return
        if (repaired.status !== 'ready' || !repaired.audio_url) return
        const blob = await training.fetchTrainingAudioFile(
          repaired.audio_url,
          signal,
        )
        if (!isCurrent()) return
        const dispose = await playQuestion(
          blob,
          () => setSpeaking(false),
          () => setSpeechError(true),
        )
        if (!isCurrent()) {
          dispose()
          return
        }
        player.current = dispose
        setAudioStatus('ready')
        setSpeechError(false)
        setSpeaking(true)
        return
      }
      if (metadata.status !== 'ready' || !metadata.audio_url) return
      const blob = await training.fetchTrainingAudioFile(
        metadata.audio_url,
        signal,
      )
      if (!isCurrent()) return
      const dispose = await playQuestion(
        blob,
        () => {
          if (isCurrent()) {
            setSpeaking(false)
            player.current = null
          }
        },
        () => {
          if (isCurrent()) {
            setSpeaking(false)
            setSpeechError(true)
            player.current = null
          }
        },
      )
      if (!isCurrent()) {
        dispose()
        return
      }
      player.current = dispose
      setAudioStatus('ready')
      setSpeaking(true)
    } catch (failure) {
      if (isCurrent()) {
        if (
          failure instanceof TrainingApiError &&
          failure.status === 401 &&
          cache.getQueryData<{ id: string } | null>(userQueryKeys.auth)?.id ===
            userId
        )
          void replaceSession(cache, null)
        onError(failure)
        setSpeechError(true)
        setAudioStatus('failed')
      }
    } finally {
      unregister()
      if (speechRequest.current === controller) speechRequest.current = null
    }
  }, [
    cache,
    exerciseId,
    onError,
    progress.session_id,
    stopSpeech,
    training,
    userId,
  ])

  const next = useCallback(() => {
    generation.current++
    lockToken.current++
    locked.current = false
    answerRequest.current?.abort()
    recording.current?.dispose()
    recording.current = null
    pending.current = null
    setAccepted(undefined)
    setStream(undefined)
    setError(undefined)
    setCaptureError('')
    disposeAnswer()
    stopSpeech()
    setStage('ready')
  }, [disposeAnswer, stopSpeech])

  return {
    stage,
    error,
    captureError,
    seconds,
    stream,
    answerUrl,
    accepted,
    audioStatus,
    speaking,
    speechError,
    hasPending: !!pending.current,
    pendingExerciseId: pending.current?.submission.exerciseId,
    start,
    stop,
    retry,
    speak,
    next,
  }
}
