import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { TrainingApi } from '@/entities/training'

export type TrainingDependencies = { training: TrainingApi }
const Context = createContext<TrainingDependencies | null>(null)

export function TrainingDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: TrainingDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}

export function useTrainingDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Training dependencies provider is missing')
  return value
}
