import { createContext, useContext } from 'react'
import type { PropsWithChildren } from 'react'
import type { AuthInput } from '@/entities/user'
import type { User } from '@/entities/user'

export type AuthDependencies = {
  mode: 'demo' | 'real'
  auth: {
    readCurrentUser(signal: AbortSignal): Promise<User | null>
    authenticate(input: AuthInput, signal: AbortSignal): Promise<User>
    logout(signal: AbortSignal): Promise<void>
  }
}
const Context = createContext<AuthDependencies | null>(null)
export function AuthDependenciesProvider({
  value,
  children,
}: PropsWithChildren<{ value: AuthDependencies }>) {
  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useAuthDependencies() {
  const value = useContext(Context)
  if (!value) throw new Error('Auth dependencies provider is missing')
  return value
}
