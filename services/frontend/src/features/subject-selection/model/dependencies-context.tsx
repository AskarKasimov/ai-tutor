import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { SubjectApi } from '@/entities/subject'

export type SubjectSelectionDependencies = { subjects: SubjectApi }
const Context = createContext<SubjectSelectionDependencies | null>(null)
export function SubjectSelectionDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: SubjectSelectionDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useSubjectSelectionDependencies() {
  const value = useContext(Context)
  if (!value)
    throw new Error('Subject selection dependencies provider is missing')
  return value
}
