import { useMutation, useQuery } from '@tanstack/react-query'

import { generateTrainingTasks, listOutcomes } from './task-generation-api'

export function useOutcomesQuery() {
  return useQuery({
    queryKey: ['outcomes'],
    queryFn: ({ signal }) => listOutcomes(signal),
  })
}

export type GenerateTrainingTasksInput = { outcomeId: string; count: number }

export function useGenerateTrainingTasks() {
  return useMutation({
    mutationFn: ({ outcomeId, count }: GenerateTrainingTasksInput) => generateTrainingTasks(outcomeId, count),
  })
}