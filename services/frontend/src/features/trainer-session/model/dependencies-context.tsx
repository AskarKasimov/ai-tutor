import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { TrainerSessionSource } from '@/entities/trainer-session'
export type TrainerSessionDependencies = {
  trainerSessionSource: TrainerSessionSource | undefined
}
const Context = createContext<TrainerSessionDependencies | null>(null)
export function TrainerSessionDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: TrainerSessionDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useTrainerSessionDependencies() {
  const value = useContext(Context)
  if (!value)
    throw new Error('Trainer session dependencies provider is missing')
  return value
}
