import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import * as audio from '../platform/prototype-audio'
import type { AssessmentTask } from '../shared/domain'
import * as assessment from './assessment-api'
import * as api from './voice-api'
import { useTrainerPrototype } from './use-trainer-prototype'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

const task: AssessmentTask = { taskId: 'ml_001', question: 'question', options: ['A', 'B'], voiceInstruction: 'instruction' }

it('starts one capture for repeated clicks and releases it when leaving', async () => {
  const dispose = vi.fn()
  const capture = vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose, stop: vi.fn() })
  const { result, unmount } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await Promise.all([result.current.start(), result.current.start()]) })
  expect(capture).toHaveBeenCalledTimes(1)
  expect(result.current.stage).toBe('recording')
  unmount()
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('finishes recording, retains audio for playback and revokes it on reset', async () => {
  vi.spyOn(api, 'transcribeRecording').mockResolvedValue({ id: 'tr-1', text: 'Ответ из API' })
  const evaluate = vi.spyOn(assessment, 'evaluateAnswer').mockResolvedValue({ score: 2, feedback: ['Верно.', 'Причина.', 'Совет.'] })
  const stop = vi.fn().mockResolvedValue(new Blob(['audio'], { type: 'audio/webm' }))
  const disposeUrl = vi.fn()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: disposeUrl })
  const { result } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.start() })
  await act(async () => { await result.current.stop() })
  expect(stop).toHaveBeenCalledTimes(1)
  expect(result.current.stage).toBe('result')
  expect(result.current.transcript).toBe('Ответ из API')
  expect(result.current.assessment?.score).toBe(2)
  expect(evaluate).toHaveBeenCalledWith('tr-1', task, expect.any(AbortSignal))
  expect(result.current.example).toBe(false)
  expect(result.current.audioUrl).toBe('blob:recording')
  await waitFor(() => expect(result.current.stage).toBe('result'), { timeout: 2500 })
  act(() => result.current.reset())
  expect(disposeUrl).toHaveBeenCalledTimes(1)
  expect(result.current.audioUrl).toBeUndefined()
  expect(result.current.stage).toBe('ready')
})

it('releases capture if microphone access completes after unmount', async () => {
  const dispose = vi.fn()
  let complete!: (capture: audio.Recording) => void
  vi.spyOn(audio, 'startRecording').mockReturnValue(new Promise((resolve) => { complete = resolve }))
  const { result, unmount } = renderHook(() => useTrainerPrototype(task))
  let pending!: Promise<void>
  act(() => { pending = result.current.start() })
  unmount()
  await act(async () => { complete({ stream: {} as MediaStream, dispose, stop: vi.fn() }); await pending })
  expect(dispose).toHaveBeenCalledTimes(1)
})


it('shows API failure instead of a demonstration result', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockRejectedValue(new Error('unavailable'))
  const { result } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.start() })
  await act(async () => { await result.current.stop() })
  expect(result.current.stage).toBe('error')
  expect(result.current.error).toBe('transcription')
  expect(result.current.transcript).toBe('')
})

it('cancels an unfinished transcription on leaving and never grades its late response', async () => {
  let complete!: (value: { id: string; text: string }) => void
  let signal!: AbortSignal
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockImplementation((_blob, incomingSignal) => {
    signal = incomingSignal
    return new Promise((resolve) => { complete = resolve })
  })
  const evaluate = vi.spyOn(assessment, 'evaluateAnswer')
  const { result, unmount } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.start() })
  let stopping!: Promise<void>
  act(() => { stopping = result.current.stop() })
  await waitFor(() => expect(signal).toBeDefined())
  unmount()
  expect(signal.aborted).toBe(true)
  await act(async () => { complete({ id: 'late', text: 'Поздний ответ' }); await stopping })
  expect(evaluate).not.toHaveBeenCalled()
})

it('keeps the transcription and retries grading without recording again', async () => {
  const capture = vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockResolvedValue({ id: 'tr-1', text: 'Мой ответ' })
  const evaluate = vi.spyOn(assessment, 'evaluateAnswer').mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ score: 1, feedback: ['Частично.', 'Есть ошибка.', 'Повторите тему.'] })
  const { result } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.start(); await result.current.stop() })
  expect(result.current.stage).toBe('error')
  expect(result.current.transcript).toBe('Мой ответ')
  await act(async () => { await result.current.retryAssessment() })
  expect(result.current.stage).toBe('result')
  expect(result.current.assessment?.score).toBe(1)
  expect(capture).toHaveBeenCalledTimes(1)
  expect(evaluate).toHaveBeenCalledTimes(2)
})

it('uses synthesized API audio and cancels playback on reset', async () => {
  const dispose = vi.fn()
  const synthesize = vi.spyOn(api, 'synthesizeQuestion').mockResolvedValue(new Blob(['wav']))
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(dispose)
  const { result } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.speak() })
  expect(synthesize.mock.calls[0][0]).toBe('instruction')
  expect(play).toHaveBeenCalledTimes(1)
  expect(result.current.speaking).toBe(true)
  act(() => result.current.reset())
  expect(dispose).toHaveBeenCalledTimes(1)
  expect(result.current.speaking).toBe(false)
})

it('speaks the selected question and exposes its recording for session playback', async () => {
  const blob = new Blob(['answer'], { type: 'audio/webm' })
  const synthesize = vi.spyOn(api, 'synthesizeQuestion').mockResolvedValue(new Blob(['wav']))
  vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(blob) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockResolvedValue({ id: 'tr-2', text: 'Регрессия' })
  vi.spyOn(assessment, 'evaluateAnswer').mockResolvedValue({ score: 2, feedback: ['Верно', 'Причина', 'Совет'] })
  const { result } = renderHook(() => useTrainerPrototype(task, 'Цена квартиры? Регрессия.'))
  await act(async () => { await result.current.speak() })
  expect(synthesize.mock.calls[0][0]).toBe('Цена квартиры? Регрессия.')
  await act(async () => { await result.current.start(); await result.current.stop() })
  expect(result.current.audioBlob).toBe(blob)
  act(() => result.current.reset())
  expect(result.current.audioBlob).toBeUndefined()
})

it('shows a session error when the backend rejects transcription or speech with 401', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockRejectedValue(new api.VoiceApiError('Transcription', 401))
  vi.spyOn(api, 'synthesizeQuestion').mockRejectedValue(new api.VoiceApiError('Synthesis', 401))
  const { result } = renderHook(() => useTrainerPrototype(task))
  await act(async () => { await result.current.speak() })
  expect(result.current.speechError).toBe('speechUnauthorized')
  await act(async () => { await result.current.start(); await result.current.stop() })
  expect(result.current.stage).toBe('error')
  expect(result.current.error).toBe('unauthorized')
})

it('exposes the active microphone stream only during recording', async () => {
  const stream = {} as MediaStream
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream, dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockRejectedValue(new Error('offline'))
  const { result } = renderHook(() => useTrainerPrototype(task))
  expect(result.current.recordingStream).toBeUndefined()
  await act(async () => { await result.current.start() })
  expect(result.current.recordingStream).toBe(stream)
  await act(async () => { await result.current.stop() })
  expect(result.current.recordingStream).toBeUndefined()
  await act(async () => { await result.current.start() })
  act(() => result.current.reset())
  expect(result.current.recordingStream).toBeUndefined()
})
