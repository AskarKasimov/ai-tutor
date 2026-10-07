import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import * as audio from '../platform/prototype-audio'
import type { Recording } from '../platform/prototype-audio'
import type { DiagnosticTask } from '../shared/domain'
import { useDiagnosticVoice } from './use-diagnostic-voice'

const task: DiagnosticTask = { variant_task_id: 't1', source_task_id: 'bank1', role: 'main', competency_id: 'c1', competency_name: 'Модели', constituent_id: 's1', constituent_name: 'Выбор', outcome_id: 'o1', outcome_name: 'Выбор модели', question: 'Вопрос', options: [], voice_instruction: 'Объясните выбор.' }
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('releases a microphone granted after the screen has been closed', async () => {
  let allow!: (recording: Recording) => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'startRecording').mockImplementation(() => new Promise((resolve) => { allow = resolve }))
  const view = renderHook(() => useDiagnosticVoice('s1', task, vi.fn(), vi.fn()))
  let starting!: Promise<void>
  act(() => { starting = view.result.current.start() })
  view.unmount()
  await act(async () => { allow({ stream: {} as MediaStream, dispose, stop: vi.fn() }); await starting })
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('aborts a pending audio submission and releases recording playback on unmount', async () => {
  const dispose = vi.fn()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({ url: 'blob:answer', dispose })
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: async () => new Blob(['audio'], { type: 'audio/webm' }) })
  let request!: AbortSignal
  const submit = vi.fn((_input, signal: AbortSignal) => {
    request = signal
    return new Promise<never>((_resolve, reject) => signal.addEventListener('abort', () => reject(signal.reason)))
  })
  const view = renderHook(() => useDiagnosticVoice('s1', task, submit, vi.fn()))
  await act(async () => { await view.result.current.start() })
  let stopping!: Promise<void>
  act(() => { stopping = view.result.current.stop() })
  await waitFor(() => expect(view.result.current.stage).toBe('processing'))
  await waitFor(() => expect(request).toBeDefined())
  view.unmount()
  await stopping
  expect(request.aborted).toBe(true)
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('cancels instruction generation when recording starts and never plays its late response', async () => {
  let speechRequest!: AbortSignal
  let resolveSpeech!: (response: Response) => void
  vi.stubGlobal('fetch', vi.fn((_url, init: RequestInit) => {
    speechRequest = init.signal!
    return new Promise((resolve) => { resolveSpeech = resolve })
  }))
  vi.spyOn(audio, 'startRecording').mockResolvedValue({ stream: {} as MediaStream, dispose: vi.fn(), stop: vi.fn() })
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderHook(() => useDiagnosticVoice('s1', task, vi.fn(), vi.fn()))
  let speaking!: Promise<void>
  act(() => { speaking = view.result.current.speak() })
  await act(async () => { await view.result.current.start() })
  await act(async () => { resolveSpeech(new Response('wav', { headers: { 'Content-Type': 'audio/wav' } })); await speaking })
  expect(speechRequest.aborted).toBe(true)
  expect(play).not.toHaveBeenCalled()
  expect(view.result.current.stage).toBe('recording')
})

it('ends instruction playback without holding a stale player after it finishes', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('wav', { headers: { 'Content-Type': 'audio/wav' } })))
  let ended!: () => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'playQuestion').mockImplementation(async (_blob, onEnd) => { ended = onEnd; return dispose })
  const view = renderHook(() => useDiagnosticVoice('s1', task, vi.fn(), vi.fn()))
  await act(async () => { await view.result.current.speak() })
  act(() => { ended() })
  expect(view.result.current.speech).toBe('idle')
  view.unmount()
  // playQuestion disposes its own URL/player on ended; the hook must drop its handle.
  expect(dispose).not.toHaveBeenCalled()
})

it('cancels a rapidly repeated instruction click instead of launching concurrent playback', async () => {
  const requests: AbortSignal[] = []
  const fetch = vi.fn((_url, init: RequestInit) => {
    requests.push(init.signal!)
    return new Promise<never>((_resolve, reject) => init.signal!.addEventListener('abort', () => reject(init.signal!.reason)))
  })
  vi.stubGlobal('fetch', fetch)
  const view = renderHook(() => useDiagnosticVoice('s1', task, vi.fn(), vi.fn()))
  await act(async () => { const first = view.result.current.speak(); await view.result.current.speak(); await first })
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(requests[0].aborted).toBe(true)
  expect(view.result.current.speech).toBe('idle')
})
