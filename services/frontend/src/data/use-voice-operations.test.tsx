import { act, renderHook } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '../app/providers'
import * as voiceApi from './voice-api'
import { useTranscriptionMutation } from './use-voice-operations'

describe('voice operation queries', () => {
  it('executes transcription as a mutation owned by TanStack Query', async () => {
    vi.spyOn(voiceApi, 'transcribeRecording').mockResolvedValue({
      id: 'tr-1',
      text: 'Ответ',
    })
    const client = createQueryClient()
    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const view = renderHook(() => useTranscriptionMutation('student'), {
      wrapper,
    })
    await act(async () => {
      await view.result.current.mutateAsync({
        blob: new Blob(['audio']),
        signal: new AbortController().signal,
      })
    })
    const operations = client
      .getMutationCache()
      .findAll({ mutationKey: ['voice', 'transcribe', 'student'] })
    expect(operations).toHaveLength(1)
    expect(operations[0].state.status).toBe('success')
    view.unmount()
    client.clear()
  })
})
