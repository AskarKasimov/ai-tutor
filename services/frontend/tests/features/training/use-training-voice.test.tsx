import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createQueryWrapper } from '../../support/query-wrapper'
import { createAppDependencies } from '@/bootstrap/dependencies'
import * as audio from '@/shared/lib'
import type { TrainingProgress } from '@/entities/training'
import { useTrainingVoice } from '@/features/training'
afterEach(() => vi.restoreAllMocks())
const progress: TrainingProgress = {
  session_id: 's1',
  diagnostic_session_id: 'd1',
  subject_id: 'sub',
  subject_name: 'Math',
  mode: 'focused',
  status: 'active',
  round: 1,
  answer_count: 0,
  targets: [],
  current: {
    exercise_id: 'e1',
    outcome_id: 'o1',
    outcome_name: 'Topic',
    question: 'Q',
    options: [],
    voice_instruction: 'Say it',
  },
}
it('retains submission on retry and clears feedback when the exercise changes', async () => {
  const deps = createAppDependencies()
  const input = {
    sessionId: 's1',
    exerciseId: 'e1',
    key: 'stable',
    body: new FormData(),
  }
  const create = vi
    .spyOn(deps.training, 'createTrainingSubmission')
    .mockReturnValue(input)
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  const release = vi.fn()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:voice',
    dispose: release,
  })
  let calls = 0
  const submit = vi.fn(
    async (submission: typeof input, signal: AbortSignal) => {
      void submission
      void signal
      if (++calls === 1) throw Error('offline')
      return progress
    },
  )
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, submit, vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
      initialProps: { current: progress },
    },
  )
  await act(async () => {
    await view.result.current.start()
    await view.result.current.stop()
  })
  expect(view.result.current.stage).toBe('error')
  await act(async () => {
    await view.result.current.retry()
  })
  expect(create).toHaveBeenCalledOnce()
  expect(submit.mock.calls[0][0]).toBe(submit.mock.calls[1][0])
  expect(view.result.current.accepted?.progress).toEqual(progress)
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  expect(release).not.toHaveBeenCalled()
  expect(view.result.current.accepted?.exercise.exercise_id).toBe('e1')
  act(() => view.result.current.next())
  expect(release).toHaveBeenCalledOnce()
  expect(view.result.current.accepted).toBeUndefined()
})
it('disposes a microphone granted after unmount', async () => {
  let grant!: (recording: audio.Recording) => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'startRecording').mockImplementation(
    () =>
      new Promise((resolve) => {
        grant = resolve
      }),
  )
  const view = renderHook(
    () => useTrainingVoice('u1', progress, vi.fn(), vi.fn()),
    { wrapper: createQueryWrapper(createQueryClient()) },
  )
  let start!: Promise<void>
  act(() => {
    start = view.result.current.start()
  })
  view.unmount()
  await act(async () => {
    grant({ stream: {} as MediaStream, dispose, stop: vi.fn() })
    await start
  })
  expect(dispose).toHaveBeenCalled()
})
it('moves through permission, recording, processing, and accepted result', async () => {
  const deps = createAppDependencies()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  vi.spyOn(deps.training, 'createTrainingSubmission').mockReturnValue({
    sessionId: 's1',
    exerciseId: 'e1',
    key: 'key',
    body: new FormData(),
  })
  const submit = vi.fn().mockResolvedValue({
    ...progress,
    answer: {
      sequence: 1,
      exercise_id: 'e1',
      round: 1,
      target_index: 0,
      transcription_id: 't1',
      text: 'answer',
      score: 1,
      max_score: 2,
      verdict: 'partial' as const,
      criterion_results: [],
      feedback: [],
      created_at: 1,
    },
  })
  const view = renderHook(
    () => useTrainingVoice('u1', progress, submit, vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
    },
  )
  let start!: Promise<void>
  act(() => {
    start = view.result.current.start()
  })
  expect(view.result.current.stage).toBe('permission')
  await act(async () => {
    await start
  })
  expect(view.result.current.stage).toBe('recording')
  let stop!: Promise<void>
  act(() => {
    stop = view.result.current.stop()
  })
  expect(view.result.current.stage).toBe('processing')
  await act(async () => {
    await stop
  })
  expect(view.result.current.stage).toBe('result')
  expect(view.result.current.accepted?.exercise.exercise_id).toBe('e1')
})
it('disposes an active capture when the exercise changes', async () => {
  const dispose = vi.fn()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose,
    stop: vi.fn(),
  })
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, vi.fn(), vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient()),
      initialProps: { current: progress },
    },
  )
  await act(async () => {
    await view.result.current.start()
  })
  expect(view.result.current.stage).toBe('recording')
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  expect(dispose).toHaveBeenCalled()
  expect(view.result.current.stage).toBe('ready')
})
it('ignores a submit result that arrives after the exercise changes', async () => {
  const deps = createAppDependencies()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  vi.spyOn(deps.training, 'createTrainingSubmission').mockReturnValue({
    sessionId: 's1',
    exerciseId: 'e1',
    key: 'key',
    body: new FormData(),
  })
  let resolve!: (value: TrainingProgress) => void
  const submit = vi.fn(
    () =>
      new Promise<TrainingProgress>((done) => {
        resolve = done
      }),
  )
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, submit, vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
      initialProps: { current: progress },
    },
  )
  await act(async () => {
    await view.result.current.start()
  })
  let stop!: Promise<void>
  act(() => {
    stop = view.result.current.stop()
  })
  expect(view.result.current.stage).toBe('processing')
  await waitFor(() => expect(submit).toHaveBeenCalledOnce())
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  await act(async () => {
    resolve(progress)
    await stop
  })
  expect(view.result.current.accepted).toBeUndefined()
  expect(view.result.current.stage).toBe('ready')
})
it('unlocks a new exercise when a prior submit resolves late', async () => {
  const deps = createAppDependencies()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  vi.spyOn(deps.training, 'createTrainingSubmission').mockReturnValue({
    sessionId: 's1',
    exerciseId: 'e1',
    key: 'key',
    body: new FormData(),
  })
  let resolve!: (value: TrainingProgress) => void
  const submit = vi.fn(
    () =>
      new Promise<TrainingProgress>((done) => {
        resolve = done
      }),
  )
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, submit, vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
      initialProps: { current: progress },
    },
  )
  await act(async () => {
    await view.result.current.start()
  })
  let stop!: Promise<void>
  act(() => {
    stop = view.result.current.stop()
  })
  await waitFor(() => expect(submit).toHaveBeenCalledOnce())
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  await act(async () => {
    resolve(progress)
    await stop
  })
  await act(async () => {
    await view.result.current.start()
  })
  expect(view.result.current.stage).toBe('recording')
})
it('releases the operation lock when microphone permission resolves after exercise change', async () => {
  let grant!: (recording: audio.Recording) => void
  const lateDispose = vi.fn()
  vi.spyOn(audio, 'startRecording')
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          grant = resolve
        }),
    )
    .mockResolvedValue({
      stream: {} as MediaStream,
      dispose: vi.fn(),
      stop: vi.fn(),
    })
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, vi.fn(), vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient()),
      initialProps: { current: progress },
    },
  )
  let firstStart!: Promise<void>
  act(() => {
    firstStart = view.result.current.start()
  })
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  await act(async () => {
    await view.result.current.start()
  })
  expect(view.result.current.stage).toBe('recording')
  await act(async () => {
    grant({ stream: {} as MediaStream, dispose: lateDispose, stop: vi.fn() })
    await firstStart
  })
  expect(lateDispose).toHaveBeenCalledOnce()
  expect(view.result.current.stage).toBe('recording')
})
it('does not fetch instruction audio while accepted feedback is displayed', async () => {
  const deps = createAppDependencies()
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn().mockResolvedValue(new Blob(['voice'])),
  })
  vi.spyOn(deps.training, 'createTrainingSubmission').mockReturnValue({
    sessionId: 's1',
    exerciseId: 'e1',
    key: 'key',
    body: new FormData(),
  })
  const readAudio = vi.spyOn(deps.training, 'readTrainingAudio')
  const submit = vi.fn().mockResolvedValue({
    ...progress,
    current: { ...progress.current, exercise_id: 'e2' },
  })
  const view = renderHook(
    () => useTrainingVoice('u1', progress, submit, vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
    },
  )
  await act(async () => {
    await view.result.current.start()
    await view.result.current.stop()
  })
  await act(async () => {
    await view.result.current.speak()
  })
  expect(view.result.current.accepted?.exercise.exercise_id).toBe('e1')
  expect(readAudio).not.toHaveBeenCalled()
})
it('ignores late instruction audio metadata after the exercise changes', async () => {
  const deps = createAppDependencies()
  let resolve!: (
    value: Awaited<ReturnType<typeof deps.training.readTrainingAudio>>,
  ) => void
  const readAudio = vi
    .spyOn(deps.training, 'readTrainingAudio')
    .mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
  const fetchAudio = vi.spyOn(deps.training, 'fetchTrainingAudioFile')
  const view = renderHook(
    ({ current }) => useTrainingVoice('u1', current, vi.fn(), vi.fn()),
    {
      wrapper: createQueryWrapper(createQueryClient(), deps),
      initialProps: { current: progress },
    },
  )
  let speak!: Promise<void>
  act(() => {
    speak = view.result.current.speak()
  })
  expect(readAudio).toHaveBeenCalledOnce()
  view.rerender({
    current: {
      ...progress,
      current: { ...progress.current, exercise_id: 'e2' },
    },
  })
  await act(async () => {
    resolve({ exercise_id: 'e1', status: 'ready', audio_url: '/audio' })
    await speak
  })
  expect(fetchAudio).not.toHaveBeenCalled()
})
