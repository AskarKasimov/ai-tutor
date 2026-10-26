import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { useReviewAudioPlayer } from '@/features/learning-diagnostic-session'
import { AudioPlaybackError } from '@/shared/lib'
import * as audio from '@/shared/lib'
import { StoredAudioError } from '@/shared/api'
import { createQueryWrapper } from '../../../support/query-wrapper'

afterEach(() => vi.restoreAllMocks())

function setup() {
  const dependencies = createAppDependencies()
  const fetch = vi.fn().mockResolvedValue(new Blob())
  dependencies.learningDiagnostic.fetchDiagnosticAudioFile = fetch
  const view = renderHook(() => useReviewAudioPlayer(), {
    wrapper: createQueryWrapper(createQueryClient(), dependencies),
  })
  return { ...view, fetch }
}

it('ignores a cancelled player error and keeps the replacement stoppable', async () => {
  let resolveOld!: (dispose: () => void) => void
  let oldError!: (error: AudioPlaybackError) => void
  let oldSignal!: AbortSignal
  const disposeOld = vi.fn()
  const disposeNew = vi.fn()
  vi.spyOn(audio, 'playQuestion')
    .mockImplementationOnce((_blob, _end, error, signal) => {
      oldError = error
      oldSignal = signal!
      return new Promise((resolve) => {
        resolveOld = resolve
      })
    })
    .mockResolvedValueOnce(disposeNew)
  const { result, fetch } = setup()
  let oldPromise!: Promise<void>
  act(() => {
    oldPromise = result.current.play('old')
  })
  await waitFor(() => expect(result.current.loadingId).toBe('old'))
  await waitFor(() => expect(audio.playQuestion).toHaveBeenCalledOnce())
  await act(async () => {
    await result.current.play('new')
  })
  expect(oldSignal.aborted).toBe(true)
  expect(fetch).toHaveBeenCalledWith(
    '/learn/task-audio/taskaudio_new/file',
    expect.any(AbortSignal),
  )
  act(() => oldError(new AudioPlaybackError('source')))
  expect(result.current.playingId).toBe('new')
  expect(result.current.notice).toBeNull()
  act(() => result.current.stop())
  expect(disposeNew).toHaveBeenCalledOnce()
  await act(async () => {
    resolveOld(disposeOld)
    await oldPromise
  })
  expect(disposeOld).toHaveBeenCalledOnce()
})

it('aborts pending playback when the review unmounts', async () => {
  let signal!: AbortSignal
  let resolvePlay!: (dispose: () => void) => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'playQuestion').mockImplementation(
    (_blob, _end, _error, inputSignal) => {
      signal = inputSignal!
      return new Promise((resolve) => {
        resolvePlay = resolve
      })
    },
  )
  const { result, unmount } = setup()
  let playing!: Promise<void>
  act(() => {
    playing = result.current.play('task')
  })
  await waitFor(() => expect(audio.playQuestion).toHaveBeenCalledOnce())
  unmount()
  expect(signal.aborted).toBe(true)
  await act(async () => {
    resolvePlay(dispose)
    await playing
  })
  expect(dispose).toHaveBeenCalledOnce()
})

it.each([
  [409, 'AUDIO_NOT_READY', 'learningTrainer.audioPreparing'],
  [404, 'TASK_AUDIO_NOT_FOUND', 'learningTrainer.audioMissing'],
  [503, 'AUDIO_STORAGE_UNAVAILABLE', 'learningTrainer.speechError'],
] as const)(
  'distinguishes stored audio fault %s / %s',
  async (status, code, key) => {
    const { result, fetch } = setup()
    fetch.mockRejectedValueOnce(new StoredAudioError('http', status, code))
    await act(async () => {
      await result.current.play('task')
    })
    expect(result.current.notice).toEqual({ taskId: 'task', key })
    expect(result.current.loadingId).toBeNull()
    const dispose = vi.fn()
    vi.spyOn(audio, 'playQuestion').mockResolvedValueOnce(dispose)
    await act(async () => {
      await result.current.play('task')
    })
    expect(result.current.notice).toBeNull()
    expect(result.current.playingId).toBe('task')
  },
)
