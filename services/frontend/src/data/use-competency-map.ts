import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import {
  CompetencyMapApiError,
  importCompetencyMap,
  readCompetencyMap,
} from './competency-map-api'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from './session-lifecycle'
import { queryKeys } from './query-keys'

export function useCompetencyMapQuery(userId: string) {
  const cache = useQueryClient()
  // Auth epoch is QueryClient-local and intentionally not part of the public resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery({
    queryKey: queryKeys.competencyMap(userId),
    enabled: !!userId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const data = await readCompetencyMap(
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

export function useImportCompetencyMapMutation(userId: string) {
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  const controllerRef = useRef<AbortController | null>(null)
  useEffect(() => () => controllerRef.current?.abort(), [])
  return useMutation({
    mutationKey: queryKeys.importCompetencyMap(userId),
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async (file: File) => {
      const controller = new AbortController()
      controllerRef.current = controller
      const unregister = registerSessionRequest(
        cache,
        tokenRef.current,
        controller,
      )
      try {
        return await importCompetencyMap(file, controller.signal)
      } finally {
        unregister()
        if (controllerRef.current === controller) controllerRef.current = null
      }
    },
    onSuccess: async () => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await cache.invalidateQueries({
        queryKey: queryKeys.competencyMap(userId),
      })
    },
    onError: async (error) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CompetencyMapApiError && error.status === 401)
        await replaceSession(cache, null)
      // The server may have committed before a timeout or unreadable response.
      await cache.invalidateQueries({
        queryKey: queryKeys.competencyMap(userId),
      })
    },
    retry: false,
  })
}

/** @deprecated Use useImportCompetencyMapMutation with the authenticated user ID. */
export function useImportCompetencyMap() {
  return useImportCompetencyMapMutation('legacy')
}
