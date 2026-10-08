import type { PropsWithChildren } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { AppProviders } from '@/bootstrap/providers'
import type { AppDependencies } from '@/bootstrap/dependencies'
import { createAppDependencies } from '@/bootstrap/dependencies'

export function createQueryWrapper(
  client: QueryClient,
  dependencies: AppDependencies = createAppDependencies(),
) {
  return function QueryWrapper({ children }: PropsWithChildren) {
    return (
      <AppProviders queryClient={client} dependencies={dependencies}>
        {children}
      </AppProviders>
    )
  }
}
