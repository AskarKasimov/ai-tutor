import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createMockTrainerSessionSource } from '../mocks/trainer-session-source'
import { createAudioUrl } from '../platform/prototype-audio'
import type { SessionAnswer } from '../shared/domain'

export function useTrainerSession(userId: string | undefined) {
  const [source] = useState(createMockTrainerSessionSource)
  const cache = useQueryClient()
  const savedAudio = useRef(new Map<string, ReturnType<typeof createAudioUrl>>())
  const generation = useRef(0)
  const queryKey = ['trainer-session', userId] as const

  function releaseAudio() {
    savedAudio.current.forEach((audio) => audio.dispose())
    savedAudio.current.clear()
  }

  useEffect(() => {
    const urls = savedAudio.current
    return () => {
      generation.current++
      urls.forEach((audio) => audio.dispose())
      urls.clear()
      if (userId) {
        source.clear(userId)
        cache.removeQueries({ queryKey: ['trainer-session', userId] })
      }
    }
  }, [userId, source, cache])

  const query = useQuery({
    queryKey, enabled: !!userId,
    queryFn: () => source.getSession(userId!),
  })
  const save = useMutation({
    mutationFn: async ({ sessionId, answer, audioBlob }: { sessionId: string; answer: SessionAnswer; audioBlob?: Blob }) => {
      if (!userId) throw new Error('No user')
      const current = generation.current
      const result = await source.saveAnswer(userId, sessionId, answer)
      if (generation.current !== current) throw new Error('Session changed')
      if (audioBlob) {
        savedAudio.current.get(answer.assignmentId)?.dispose()
        savedAudio.current.set(answer.assignmentId, createAudioUrl(audioBlob))
      }
      return result
    },
    onSuccess: () => cache.invalidateQueries({ queryKey }),
  })
  const restart = useMutation({
    mutationFn: async () => {
      if (!userId) throw new Error('No user')
      generation.current++
      releaseAudio()
      return source.restart(userId)
    },
    onSuccess: () => cache.invalidateQueries({ queryKey }),
  })

  return { query, save, restart, audioUrl: (assignmentId: string) => savedAudio.current.get(assignmentId)?.url }
}
