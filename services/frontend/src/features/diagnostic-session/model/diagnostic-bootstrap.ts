import type { QueryClient } from '@tanstack/react-query'
import { createDiagnosticBootstrapCoordinator } from '@/features/diagnostic-session/model/bootstrap-coordinator'
import type { DiagnosticSessionIdentity } from '@/entities/diagnostic-session'
import type { SessionToken } from '@/entities/user'
import type { DiagnosticProgress } from '@/entities/diagnostic-session'

const coordinators = new WeakMap<
  QueryClient,
  ReturnType<typeof createDiagnosticBootstrapCoordinator>
>()
export function acquireDiagnosticBootstrap(
  client: QueryClient,
  token: SessionToken,
  identity: DiagnosticSessionIdentity,
  run: (
    signal: AbortSignal,
    identity: DiagnosticSessionIdentity,
  ) => Promise<DiagnosticProgress>,
) {
  let coordinator = coordinators.get(client)
  if (!coordinator) {
    coordinator = createDiagnosticBootstrapCoordinator()
    coordinators.set(client, coordinator)
  }
  return coordinator.acquire(token, identity, run)
}
