import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { CourseGraphApiError } from '@/entities/course-graph'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { useCourseGraphBuildDependencies } from './dependencies-context'
import { courseGraphQueryKeys } from './query-keys'
import { subjectQueryKeys } from '@/entities/subject'

export function useLatestCourseGraphBuildQuery(
  userId: string,
  subjectId: string,
) {
  const { courseGraph } = useCourseGraphBuildDependencies()
  const cache = useQueryClient()
  const lastSuccessRef = useRef<string | null>(null)
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- auth epoch and dependency port are scoped outside the public resource key.
  const query = useQuery({
    queryKey: courseGraphQueryKeys.build(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const build = await courseGraph.readLatestCourseGraphBuild(
          subjectId,
          AbortSignal.any([signal, controller.signal]),
        )
        if (!isCurrentSession(cache, token) || token.userId !== userId)
          throw new DOMException('Stale session', 'AbortError')
        return build
      } catch (error) {
        if (
          error instanceof CourseGraphApiError &&
          error.status === 401 &&
          isCurrentSession(cache, token)
        )
          await replaceSession(cache, null)
        throw error
      } finally {
        unregister()
      }
    },
    refetchInterval: (current) =>
      current.state.data?.status === 'queued' ||
      current.state.data?.status === 'running'
        ? 2000
        : false,
  })
  useEffect(() => {
    if (query.data?.status !== 'succeeded') return
    if (lastSuccessRef.current === query.data.id) return
    lastSuccessRef.current = query.data.id
    void cache.invalidateQueries({
      queryKey: courseGraphQueryKeys.graph(userId, subjectId),
    })
    void cache.invalidateQueries({
      queryKey: subjectQueryKeys.subjects(userId),
    })
  }, [cache, query.data, subjectId, userId])
  return query
}

export function useCourseGraphQuery(userId: string, subjectId: string) {
  const { courseGraph } = useCourseGraphBuildDependencies()
  const cache = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- auth epoch and dependency port are scoped outside the public resource key.
  return useQuery({
    queryKey: courseGraphQueryKeys.graph(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const graph = await courseGraph.readCourseGraph(
          subjectId,
          AbortSignal.any([signal, controller.signal]),
        )
        if (!isCurrentSession(cache, token) || token.userId !== userId)
          throw new DOMException('Stale session', 'AbortError')
        return graph
      } catch (error) {
        if (
          error instanceof CourseGraphApiError &&
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

export function useStartCourseGraphBuildMutation(
  userId: string,
  subjectId: string,
) {
  const { courseGraph } = useCourseGraphBuildDependencies()
  const cache = useQueryClient()
  const tokenRef = useRef(captureSession(cache))
  return useMutation({
    mutationKey: ['course-graph-build', 'start', userId, subjectId],
    gcTime: 0,
    onMutate: () => {
      tokenRef.current = captureSession(cache)
    },
    mutationFn: async (targetSubjectId: string) => {
      const controller = new AbortController()
      const unregister = registerSessionRequest(
        cache,
        tokenRef.current,
        controller,
      )
      try {
        return await courseGraph.startCourseGraphBuild(
          targetSubjectId,
          controller.signal,
        )
      } finally {
        unregister()
      }
    },
    onSuccess: async (_data, targetSubjectId) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      await Promise.all([
        cache.invalidateQueries({
          queryKey: courseGraphQueryKeys.build(userId, targetSubjectId),
        }),
        cache.invalidateQueries({
          queryKey: subjectQueryKeys.subjects(userId),
        }),
      ])
    },
    onError: async (error, targetSubjectId) => {
      if (!isCurrentSession(cache, tokenRef.current)) return
      if (error instanceof CourseGraphApiError && error.status === 401)
        await replaceSession(cache, null)
      await cache.invalidateQueries({
        queryKey: courseGraphQueryKeys.build(userId, targetSubjectId),
      })
    },
    retry: false,
  })
}
