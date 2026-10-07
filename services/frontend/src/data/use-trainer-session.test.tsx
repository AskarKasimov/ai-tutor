import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '../app/providers'
import * as audio from '../platform/prototype-audio'
import { useTrainerSession } from './use-trainer-session'

afterEach(() => vi.restoreAllMocks())

function setup() {
  const queryClient = createQueryClient()
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return renderHook(({ userId }) => useTrainerSession(userId), {
    wrapper,
    initialProps: { userId: 'one' as string | undefined },
  })
}

it('retains saved audio until restart and rejects results belonging to the old session', async () => {
  const dispose = vi.fn()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:saved',
    dispose,
  })
  const { result } = setup()
  await waitFor(() => expect(result.current.query.data?.tasks).toHaveLength(7))
  const session = result.current.query.data!
  const input = {
    sessionId: session.id,
    answer: {
      assignmentId: session.tasks[0].assignmentId,
      transcript: 'Ответ',
      assessment: {
        score: 2 as const,
        feedback: ['Верно', 'Причина', 'Совет'] as [string, string, string],
      },
      submittedAt: 1791200000,
    },
    audioBlob: new Blob(['audio']),
  }
  await act(async () => {
    await result.current.save.mutateAsync(input)
  })
  await waitFor(() =>
    expect(result.current.query.data?.answers).toHaveLength(1),
  )
  expect(result.current.audioUrl(input.answer.assignmentId)).toBe('blob:saved')
  expect(dispose).not.toHaveBeenCalled()
  await act(async () => {
    await result.current.restart.mutateAsync()
  })
  await waitFor(() =>
    expect(result.current.query.data?.answers).toHaveLength(0),
  )
  expect(dispose).toHaveBeenCalledTimes(1)
  await act(async () => {
    await expect(result.current.save.mutateAsync(input)).rejects.toThrow()
  })
  expect(result.current.query.data?.answers).toHaveLength(0)
})

it('clears results and releases saved audio on logout, and starts the next user empty', async () => {
  const dispose = vi.fn()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:saved',
    dispose,
  })
  const { result, rerender } = setup()
  await waitFor(() => expect(result.current.query.data).toBeDefined())
  const session = result.current.query.data!
  await act(async () => {
    await result.current.save.mutateAsync({
      sessionId: session.id,
      answer: {
        assignmentId: session.tasks[0].assignmentId,
        transcript: 'Личный ответ',
        assessment: { score: 1, feedback: ['Частично', 'Причина', 'Совет'] },
        submittedAt: 1791200000,
      },
      audioBlob: new Blob(['audio']),
    })
  })
  rerender({ userId: undefined })
  expect(dispose).toHaveBeenCalledTimes(1)
  expect(result.current.query.data).toBeUndefined()
  rerender({ userId: 'two' })
  await waitFor(() => expect(result.current.query.data?.userId).toBe('two'))
  expect(result.current.query.data?.answers).toEqual([])
})
