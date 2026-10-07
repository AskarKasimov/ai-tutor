import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CompetencyMapApiError, importCompetencyMap, readCompetencyMap } from './competency-map-api'

const key = ['competency-map'] as const

export function useCompetencyMapQuery() {
  const cache = useQueryClient()
  const query = useQuery({ queryKey: key, queryFn: ({ signal }) => readCompetencyMap(signal) })
  useEffect(() => {
    if (query.error instanceof CompetencyMapApiError && query.error.status === 401) cache.setQueryData(['auth', 'me'], null)
  }, [query.error, cache])
  return query
}

export function useImportCompetencyMap() {
  const cache = useQueryClient()
  return useMutation({
    mutationFn: importCompetencyMap,
    retry: false,
    onSuccess: () => cache.invalidateQueries({ queryKey: key }),
    onError: (error) => {
      if (error instanceof CompetencyMapApiError && error.status === 401) cache.setQueryData(['auth', 'me'], null)
      // A timeout or unreadable response may follow a committed import. Re-read before another replacement.
      void cache.invalidateQueries({ queryKey: key })
    },
  })
}
