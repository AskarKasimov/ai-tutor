import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { CompetencyMapApiError } from '@/entities/competency-map'
import { useCompetencyMapDependencies } from './dependencies-context'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { competencyMapQueryKeys } from './query-keys'
import { subjectQueryKeys } from '@/entities/subject'

export function useCompetencyMapQuery(userId: string, subjectId: string) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  // Auth epoch is QueryClient-local and intentionally not part of the public resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery({
    queryKey: competencyMapQueryKeys.competencyMap(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const data = await competencyMap.read(
          subjectId,
          AbortSignal.any([signal, controller.signal]),
        )
        if (!isCurrentSession(cache, token) || token.userId !== userId)
          throw new DOMException('Stale session', 'AbortError')
        return data
      } catch (error) {
        if (
          error instanceof CompetencyMapApiError &&
          error.status === 401 &&
          isCurrentSession(cache, token)
        )
          await replaceSession(cache, null)
        throw error
      } finally {
        unregister()
      }
    },
  })
}

export function useImportCompetencyMapMutation(
  userId: string,
  subjectId: string,
) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  const controllerRef = useRef<AbortController | null>(null)
  useEffect(() => () => controllerRef.current?.abort(), [subjectId])
  return useMutation({
    mutationKey: competencyMapQueryKeys.importCompetencyMap(userId, subjectId),
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async ({
      file,
      subjectId: targetSubjectId,
    }: {
      file: File
      subjectId: string
    }) => {
      const controller = new AbortController()
      controllerRef.current = controller
      const unregister = registerSessionRequest(
        cache,
        tokenRef.current,
        controller,
      )
      try {
        return await competencyMap.import(
          targetSubjectId,
          file,
          controller.signal,
        )
      } finally {
        unregister()
        if (controllerRef.current === controller) controllerRef.current = null
      }
    },
    onSuccess: async (_data, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await invalidateTarget(variables.subjectId)
    },
    onError: async (error, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CompetencyMapApiError && error.status === 401)
        await replaceSession(cache, null)
      // The server may have committed before a timeout or unreadable response.
      await invalidateTarget(variables.subjectId)
    },
    retry: false,
  })

  async function invalidateTarget(targetSubjectId: string) {
    await Promise.all([
      cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.competencyMap(userId, targetSubjectId),
      }),
      cache.invalidateQueries({ queryKey: subjectQueryKeys.subjects(userId) }),
      cache.invalidateQueries({
        queryKey: subjectQueryKeys.learningState(userId, targetSubjectId),
      }),
    ])
  }
}
