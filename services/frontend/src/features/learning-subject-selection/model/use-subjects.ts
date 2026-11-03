import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useSubjectSelectionDependencies } from './dependencies-context'
import { subjectSelectionQueryKeys } from './query-keys'
import { SubjectApiError } from '@/entities/learning-subject'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'

async function fetchScoped<T>(
  userId: string,
  signal: AbortSignal,
  request: (signal: AbortSignal) => Promise<T>,
  client: ReturnType<typeof useQueryClient>,
) {
  const token = captureSession(client)
  const controller = new AbortController()
  const unregister = registerSessionRequest(client, token, controller)
  try {
    const value = await request(AbortSignal.any([signal, controller.signal]))
    if (!isCurrentSession(client, token) || token.userId !== userId)
      throw new DOMException('Stale session', 'AbortError')
    return value
  } catch (error) {
    if (
      error instanceof SubjectApiError &&
      error.status === 401 &&
      isCurrentSession(client, token)
    )
      await replaceSession(client, null)
    throw error
  } finally {
    unregister()
  }
}

export function useSubjectsQuery(userId: string) {
  const { subjects } = useSubjectSelectionDependencies()
  const client = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the API port and auth epoch are stable dependencies outside the resource key.
  return useQuery({
    queryKey: subjectSelectionQueryKeys.subjects(userId),
    enabled: !!userId,
    queryFn: ({ signal }) =>
      fetchScoped(userId, signal, subjects.listSubjects, client),
  })
}
export function useLearningStateQuery(userId: string, subjectId: string) {
  const { subjects } = useSubjectSelectionDependencies()
  const client = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the API port and auth epoch are stable dependencies outside the resource key.
  return useQuery({
    queryKey: subjectSelectionQueryKeys.learningState(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: ({ signal }) =>
      fetchScoped(
        userId,
        signal,
        (requestSignal) => subjects.readLearningState(subjectId, requestSignal),
        client,
      ),
  })
}
