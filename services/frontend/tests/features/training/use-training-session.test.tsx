import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createQueryWrapper } from '../../support/query-wrapper'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { TrainingApiError } from '@/entities/training'
import type { TrainingProgress, TrainingPreview } from '@/entities/training'
import {
  useStartTrainingMutation,
  useTrainingEntryQuery,
  useTrainingSession,
} from '@/features/training'
import { replaceSession } from '@/entities/user'
const user = {
  id: 'u1',
  email: 'u@example.com',
  display_name: null,
  role: 'student' as const,
}
const preview: TrainingPreview = {
  diagnostic_session_id: 'd1',
  subject_id: 'sub1',
  subject_name: 'Math',
  mode: 'focused',
  plan_revision: 2,
  diagnostic_score: 1,
  maximum_score: 2,
  status: 'ready',
  confirmed_gaps: [],
  partial_competencies: [],
  topics: [],
}
const progress: TrainingProgress = {
  session_id: 's1',
  diagnostic_session_id: 'd1',
  subject_id: 'sub1',
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
    question: 'Question',
    options: [],
    voice_instruction: 'Answer',
  },
}
afterEach(() => vi.restoreAllMocks())
function setup() {
  const client = createQueryClient()
  const deps = createAppDependencies()
  return { client, deps, wrapper: createQueryWrapper(client, deps) }
}
it('looks up existing training before preview and resumes frozen targets', async () => {
  const { client, deps, wrapper } = setup()
  await replaceSession(client, user)
  const find = vi
    .fn()
    .mockRejectedValue(new TrainingApiError(404, 'TRAINING_NOT_FOUND'))
  const read = vi.fn().mockResolvedValue(preview)
  deps.training.findTrainingForDiagnostic = find
  deps.training.readTrainingPreview = read
  const view = renderHook(() => useTrainingEntryQuery(user.id, 'd1'), {
    wrapper,
  })
  await waitFor(() => expect(view.result.current.data?.kind).toBe('preview'))
  expect(find).toHaveBeenCalledOnce()
  expect(read).toHaveBeenCalledOnce()
  find.mockResolvedValue({
    ...progress,
    targets: [
      {
        kind: 'frozen',
        label: 'x',
        competency_id: 'c',
        competency_name: 'Frozen',
        outcome_id: 'o',
        outcome_name: 'Old topic',
        original_score: null,
        original_max_score: null,
        last_score: null,
      },
    ],
  })
  await act(async () => {
    await view.result.current.refetch()
  })
  await waitFor(() => expect(view.result.current.data?.kind).toBe('existing'))
  expect(read).toHaveBeenCalledOnce()
})
it('retains the start key on retry and writes accepted answers into the session cache', async () => {
  const { client, deps, wrapper } = setup()
  await replaceSession(client, user)
  const start = vi
    .fn()
    .mockRejectedValueOnce(Error('network'))
    .mockResolvedValue(progress)
  deps.training.startTraining = start
  const view = renderHook(() => useStartTrainingMutation(user.id, 'd1'), {
    wrapper,
  })
  await act(async () => {
    await view.result.current.mutateAsync({ preview }).catch(() => {})
  })
  await act(async () => {
    await view.result.current.mutateAsync({ preview })
  })
  expect(start.mock.calls[0][1]).toBe(start.mock.calls[1][1])
  const accepted = {
    ...progress,
    answer_count: 1,
    round: 2,
    answer: {
      sequence: 1,
      exercise_id: 'e1',
      round: 1,
      target_index: 0,
      transcription_id: 't1',
      text: 'Answer',
      score: 2,
      max_score: 2 as const,
      verdict: 'correct' as const,
      criterion_results: [],
      feedback: ['ok'],
      created_at: 1,
    },
  }
  deps.training.readTrainingSession = vi.fn().mockResolvedValue(progress)
  deps.training.submitTraining = vi.fn().mockResolvedValue(accepted)
  const session = renderHook(() => useTrainingSession(user.id, 's1'), {
    wrapper,
  })
  await waitFor(() =>
    expect(session.result.current.query.data).toEqual(progress),
  )
  const submission = deps.training.createTrainingSubmission(
    's1',
    'e1',
    new Blob(['audio']),
  )
  await act(async () => {
    await session.result.current.submit.mutateAsync({
      submission,
      signal: new AbortController().signal,
    })
  })
  await waitFor(() =>
    expect(session.result.current.query.data?.answer?.exercise_id).toBe('e1'),
  )
})
it('refreshes a stale preview and requires a new explicit start with a fresh key', async () => {
  const { client, deps, wrapper } = setup()
  await replaceSession(client, user)
  const start = vi
    .fn()
    .mockRejectedValueOnce(new TrainingApiError(409, 'PLAN_CHANGED'))
    .mockResolvedValue(progress)
  deps.training.startTraining = start
  const view = renderHook(() => useStartTrainingMutation(user.id, 'd1'), {
    wrapper,
  })
  await act(async () => {
    await view.result.current.mutateAsync({ preview }).catch(() => undefined)
  })
  await act(async () => {
    await view.result.current.mutateAsync({
      preview: { ...preview, mode: 'free_practice', plan_revision: 3 },
    })
  })
  expect(start.mock.calls[0][1]).not.toBe(start.mock.calls[1][1])
  expect(start.mock.calls[1][2]).toBe(3)
})
it('ignores late session data after the authenticated epoch changes', async () => {
  const { client, deps, wrapper } = setup()
  await replaceSession(client, user)
  let finish!: (value: TrainingProgress) => void
  deps.training.readTrainingSession = vi.fn().mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  const view = renderHook(() => useTrainingSession(user.id, 's1'), { wrapper })
  await waitFor(() => expect(finish).toBeDefined())
  await act(async () => {
    await replaceSession(client, null)
    await replaceSession(client, user)
  })
  finish({ ...progress, answer_count: 99 })
  await act(async () => {
    await Promise.resolve()
  })
  expect(view.result.current.query.data?.answer_count).not.toBe(99)
})
