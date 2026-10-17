import {
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import {
  CompetencyMapApiError,
  type CourseGraphSnapshot,
  type CourseMaterial,
} from '@/entities/competency-map'
import { useCompetencyMapDependencies } from './dependencies-context'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { competencyMapQueryKeys } from './query-keys'

const pollMs = 4000

// A material is still moving through OCR, translation or cleaning.
export function materialInProgress(material: CourseMaterial): boolean {
  const active = (status?: string) =>
    status === 'pending' || status === 'processing'
  if (material.status === 'failed') return false
  return (
    material.status === 'pending' ||
    active(material.processing?.status) ||
    active(material.cleaning?.status)
  )
}

// Materials are polled until each one reaches a terminal state.
export function materialsRefetchInterval(
  materials: CourseMaterial[] | undefined,
): number | false {
  return materials?.some(materialInProgress) ? pollMs : false
}

// The graph is polled while a build runs and while materials are processed,
// so inputs_ready and stale follow them without a manual reload.
export function courseGraphRefetchInterval(
  snapshot: CourseGraphSnapshot | undefined,
  materialsBusy: boolean,
): number | false {
  const status = snapshot?.status
  return status === 'pending' || status === 'processing' || materialsBusy
    ? pollMs
    : false
}

export function useSubjectMaterialsQuery(userId: string, subjectId: string) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  // Auth epoch is QueryClient-local and intentionally not part of the public resource key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery({
    queryKey: competencyMapQueryKeys.subjectMaterials(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const data = await competencyMap.readSubjectMaterials(
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
    refetchInterval: (query) => materialsRefetchInterval(query.state.data),
  })
}

export function useUploadCourseMaterialsMutation(
  userId: string,
  subjectId: string,
) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  const controllerRef = useRef<AbortController | null>(null)
  useEffect(() => () => controllerRef.current?.abort(), [subjectId])
  return useMutation({
    mutationKey: competencyMapQueryKeys.uploadSubjectMaterials(
      userId,
      subjectId,
    ),
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async ({
      files,
      subjectId: targetSubjectId,
    }: {
      files: File[]
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
        return await competencyMap.uploadCourseMaterials(
          targetSubjectId,
          files,
          controller.signal,
        )
      } finally {
        unregister()
        if (controllerRef.current === controller) controllerRef.current = null
      }
    },
    onSuccess: async (_data, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(
          userId,
          variables.subjectId,
        ),
      })
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.courseGraph(
          userId,
          variables.subjectId,
        ),
      })
    },
    onError: async (error, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CompetencyMapApiError && error.status === 401)
        await replaceSession(cache, null)
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(
          userId,
          variables.subjectId,
        ),
      })
    },
    retry: false,
  })
}

export function useDeleteCourseMaterialMutation(
  userId: string,
  subjectId: string,
) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  return useMutation({
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async ({
      materialId,
      subjectId: targetSubjectId,
    }: {
      materialId: string
      subjectId?: string
    }) => {
      return await competencyMap.deleteCourseMaterial(
        targetSubjectId || subjectId,
        materialId,
      )
    },
    onSuccess: async (_data, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      const targetId = variables.subjectId || subjectId
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(userId, targetId),
      })
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.courseGraph(userId, targetId),
      })
    },
    retry: false,
  })
}

export function useCourseGraphQuery(
  userId: string,
  subjectId: string,
  materialsBusy: boolean,
) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery({
    queryKey: competencyMapQueryKeys.courseGraph(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const data = await competencyMap.readCourseGraph(
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
    refetchInterval: (query) =>
      courseGraphRefetchInterval(query.state.data, materialsBusy),
  })
}

export function useBuildCourseGraphMutation(userId: string, subjectId: string) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  return useMutation({
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async () => {
      const controller = new AbortController()
      const unregister = registerSessionRequest(
        cache,
        tokenRef.current,
        controller,
      )
      try {
        return await competencyMap.requestCourseGraphBuild(
          subjectId,
          controller.signal,
        )
      } finally {
        unregister()
      }
    },
    onSuccess: (snapshot) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      cache.setQueryData(
        competencyMapQueryKeys.courseGraph(userId, subjectId),
        snapshot,
      )
    },
    onError: async (error) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CompetencyMapApiError && error.status === 401)
        await replaceSession(cache, null)
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.courseGraph(userId, subjectId),
      })
    },
    retry: false,
  })
}

export function useCourseMaterialsChanging(userId: string, subjectId: string) {
  return (
    useIsMutating({
      mutationKey: competencyMapQueryKeys.uploadSubjectMaterials(
        userId,
        subjectId,
      ),
    }) > 0
  )
}
