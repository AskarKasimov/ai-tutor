import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import * as audio from '../platform/prototype-audio'
import * as api from './voice-api'
import { useTrainerPrototype } from './use-trainer-prototype'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('starts one capture for repeated clicks and releases it when leaving', async () => {
  const dispose = vi.fn()
  const capture = vi.spyOn(audio, 'startRecording').mockResolvedValue({ dispose, stop: vi.fn() })
  const { result, unmount } = renderHook(() => useTrainerPrototype('question'))
  await act(async () => { await Promise.all([result.current.start(), result.current.start()]) })
  expect(capture).toHaveBeenCalledTimes(1)
  expect(result.current.stage).toBe('recording')
  unmount()
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('finishes recording, retains audio for playback and revokes it on reset', async () => {
  vi.spyOn(api, 'transcribeRecording').mockResolvedValue('Ответ из API')
  const stop = vi.fn().mockResolvedValue(new Blob(['audio'], { type: 'audio/webm' }))
  const disposeUrl = vi.fn()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ dispose: vi.fn(), stop })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: disposeUrl })
  const { result } = renderHook(() => useTrainerPrototype('question'))
  await act(async () => { await result.current.start() })
  await act(async () => { await result.current.stop() })
  expect(stop).toHaveBeenCalledTimes(1)
  expect(result.current.stage).toBe('result')
  expect(result.current.transcript).toBe('Ответ из API')
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
  const { result, unmount } = renderHook(() => useTrainerPrototype('question'))
  let pending!: Promise<void>
  act(() => { pending = result.current.start() })
  unmount()
  await act(async () => { complete({ dispose, stop: vi.fn() }); await pending })
  expect(dispose).toHaveBeenCalledTimes(1)
})


it('shows API failure instead of a demonstration result', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockRejectedValue(new Error('unavailable'))
  const { result } = renderHook(() => useTrainerPrototype('question'))
  await act(async () => { await result.current.start() })
  await act(async () => { await result.current.stop() })
  expect(result.current.stage).toBe('error')
  expect(result.current.error).toBe('transcription')
  expect(result.current.transcript).toBe('')
})

it('uses synthesized API audio and cancels playback on reset', async () => {
  const dispose = vi.fn()
  const synthesize = vi.spyOn(api, 'synthesizeQuestion').mockResolvedValue(new Blob(['wav']))
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(dispose)
  const { result } = renderHook(() => useTrainerPrototype('Question text'))
  await act(async () => { await result.current.speak() })
  expect(synthesize.mock.calls[0][0]).toBe('Question text')
  expect(play).toHaveBeenCalledTimes(1)
  expect(result.current.speaking).toBe(true)
  act(() => result.current.reset())
  expect(dispose).toHaveBeenCalledTimes(1)
  expect(result.current.speaking).toBe(false)
})

it('shows a session error when the backend rejects transcription or speech with 401', async () => {
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ dispose: vi.fn(), stop: vi.fn().mockResolvedValue(new Blob(['audio'])) })
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:recording', dispose: vi.fn() })
  vi.spyOn(api, 'transcribeRecording').mockRejectedValue(new api.VoiceApiError('Transcription', 401))
  vi.spyOn(api, 'synthesizeQuestion').mockRejectedValue(new api.VoiceApiError('Synthesis', 401))
  const { result } = renderHook(() => useTrainerPrototype('question'))
  await act(async () => { await result.current.speak() })
  expect(result.current.speechError).toBe('speechUnauthorized')
  await act(async () => { await result.current.start(); await result.current.stop() })
  expect(result.current.stage).toBe('error')
  expect(result.current.error).toBe('unauthorized')
})
