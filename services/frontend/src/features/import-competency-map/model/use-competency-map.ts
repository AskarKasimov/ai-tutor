import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { CompetencyMapApiError } from '@/entities/competency-map'
import type {
  CourseMaterialMetadataPatch,
  CourseMaterialUploadItem,
} from '@/entities/competency-map'
import { useCompetencyMapDependencies } from './dependencies-context'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { competencyMapQueryKeys } from './query-keys'

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
      items,
      subjectId: targetSubjectId,
    }: {
      items: CourseMaterialUploadItem[]
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
          items,
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

export function useReplaceCourseMaterialMutation(
  userId: string,
  subjectId: string,
) {
  const { competencyMap } = useCompetencyMapDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  const controllerRef = useRef<AbortController | null>(null)
  useEffect(() => () => controllerRef.current?.abort(), [subjectId])
  return useMutation({
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async ({
      materialId,
      file,
    }: {
      materialId: string
      file: File
    }) => {
      const controller = new AbortController()
      controllerRef.current = controller
      const unregister = registerSessionRequest(
        cache,
        tokenRef.current,
        controller,
      )
      try {
        return await competencyMap.replaceCourseMaterialFile(
          subjectId,
          materialId,
          file,
          controller.signal,
        )
      } finally {
        unregister()
        if (controllerRef.current === controller) controllerRef.current = null
      }
    },
    onSuccess: async () => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(userId, subjectId),
      })
    },
    onError: async (error) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CompetencyMapApiError && error.status === 401)
        await replaceSession(cache, null)
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(userId, subjectId),
      })
    },
    retry: false,
  })
}

export function useUpdateCourseMaterialMetadataMutation(
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
      patch,
      subjectId: targetSubjectId,
    }: {
      materialId: string
      patch: CourseMaterialMetadataPatch
      subjectId?: string
    }) =>
      competencyMap.updateCourseMaterialMetadata(
        targetSubjectId || subjectId,
        materialId,
        patch,
      ),
    onSuccess: async (_data, variables) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(
          userId,
          variables.subjectId || subjectId,
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
          variables.subjectId || subjectId,
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
      await cache.invalidateQueries({
        queryKey: competencyMapQueryKeys.subjectMaterials(
          userId,
          variables.subjectId || subjectId,
        ),
      })
    },
    retry: false,
  })
}
