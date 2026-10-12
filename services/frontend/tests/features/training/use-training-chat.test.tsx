import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import type {
  TrainingChatPage,
  TrainingChatTurnResult,
} from '@/entities/training'
import { TrainingApiError } from '@/entities/training'
import { replaceSession } from '@/entities/user'
import { useTrainingChat } from '@/features/training'
import { createQueryWrapper } from '../../support/query-wrapper'

const user = {
  id: 'chat-user',
  email: 'student@example.com',
  display_name: null,
  role: 'student' as const,
}
const page: TrainingChatPage = {
  session_id: 'session-1',
  exercise_id: 'exercise-1',
  revision: 0,
  status: 'active',
  items: [
    {
      id: 'welcome',
      sequence: 1,
      role: 'assistant',
      kind: 'welcome',
      text: 'What do you know?',
      created_at: 1,
    },
  ],
  latest_check: null,
  pending_turn: null,
}
const response: TrainingChatTurnResult = {
  session_id: 'session-1',
  exercise_id: 'exercise-1',
  revision: 1,
  turn_id: 'turn-1',
  messages: [
    {
      id: 'user-message',
      sequence: 2,
      role: 'user',
      kind: 'message',
      source: 'text',
      text: 'I have an idea',
      created_at: 2,
    },
    {
      id: 'assistant-message',
      sequence: 3,
      role: 'assistant',
      kind: 'message',
      text: 'What supports it?',
      created_at: 2,
    },
  ],
  latest_check: null,
}

afterEach(() => vi.restoreAllMocks())

it('retains the same idempotency key for an exact send retry', async () => {
  const client = createQueryClient()
  const dependencies = createAppDependencies()
  await replaceSession(client, user)
  dependencies.training.readTrainingChat = vi
    .fn()
    .mockResolvedValueOnce(page)
    .mockResolvedValue({
      ...page,
      revision: 1,
      items: [...page.items, ...response.messages],
    })
  const send = vi
    .fn()
    .mockRejectedValueOnce(new TrainingApiError(504, 'TUTOR_TIMEOUT'))
    .mockResolvedValue(response)
  dependencies.training.sendTrainingChatText = send
  const view = renderHook(
    () => useTrainingChat(user.id, 'session-1', 'exercise-1'),
    {
      wrapper: createQueryWrapper(client, dependencies),
    },
  )
  await waitFor(() =>
    expect(view.result.current.query.data?.pages[0]?.revision).toBe(0),
  )
  const submission = {
    kind: 'message' as const,
    source: 'text' as const,
    revision: 0,
    text: 'I have an idea',
  }
  await act(async () => {
    await view.result.current.send.mutateAsync(submission).catch(() => {})
  })
  await waitFor(() =>
    expect(dependencies.training.readTrainingChat).toHaveBeenCalledTimes(2),
  )
  await act(async () => {
    await view.result.current.send.mutateAsync(submission)
  })
  expect(send).toHaveBeenCalledTimes(2)
  expect(send.mock.calls[0]?.[2].key).toBe(send.mock.calls[1]?.[2].key)
  await waitFor(() =>
    expect(view.result.current.query.data?.pages[0]?.revision).toBe(1),
  )
})
