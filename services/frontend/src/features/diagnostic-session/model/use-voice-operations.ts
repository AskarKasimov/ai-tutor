import { useQuery } from '@tanstack/react-query'
import { useDiagnosticDependencies } from './dependencies-context'
import { diagnosticSessionQueryKeys } from './query-keys'

export function useDiagnosticAudioQuery(
  userId: string,
  sessionId: string,
  taskId: string | undefined,
) {
  const { diagnostic } = useDiagnosticDependencies()
  return useQuery({
    queryKey: diagnosticSessionQueryKeys.diagnosticAudio(
      userId,
      sessionId,
      taskId,
    ),
    queryFn: ({ signal }) =>
      taskId
        ? diagnostic.readDiagnosticAudio(sessionId, taskId, signal)
        : Promise.reject(new Error('Current task is missing')),
    enabled: false,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}
