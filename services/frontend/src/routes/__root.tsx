import { createRootRouteWithContext } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { RootShell } from '@/bootstrap/root-shell'

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()(
  {
    component: RootComponent,
  },
)

function RootComponent() {
  const { queryClient } = Route.useRouteContext()
  return <RootShell queryClient={queryClient} />
}
