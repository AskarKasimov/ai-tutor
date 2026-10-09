import { act, renderHook } from '@testing-library/react'
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
  expect(dispose).toHaveBeenCalledOnce()
})
