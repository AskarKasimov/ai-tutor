import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { LearningPlanApi } from '@/entities/learning-plan'

export type LearningPlanDependencies = { learningPlan: LearningPlanApi }
const Context = createContext<LearningPlanDependencies | null>(null)

export function LearningPlanDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: LearningPlanDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}

export function useLearningPlanDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Learning plan dependencies provider is missing')
  return value
}
