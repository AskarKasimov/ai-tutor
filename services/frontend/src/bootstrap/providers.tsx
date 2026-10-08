import { Theme } from '@radix-ui/themes'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createContext, useContext, useState } from 'react'
import type { PropsWithChildren } from 'react'
import { AuthDependenciesProvider } from '@/features/auth'
import { CompetencyMapDependenciesProvider } from '@/features/import-competency-map'
import { DiagnosticDependenciesProvider } from '@/features/diagnostic-session'
import { TrainerSessionDependenciesProvider } from '@/features/trainer-session'
import { VoiceAnswerDependenciesProvider } from '@/features/voice-answer'
import { createAppDependencies } from './dependencies'
import type { AppDependencies } from './dependencies'
import '@/shared/i18n'
import styles from './providers.module.scss'

const BootstrapMode = createContext<'demo' | 'real' | null>(null)
export function useBootstrapMode() {
  const mode = useContext(BootstrapMode)
  if (!mode) throw new Error('Bootstrap mode provider is missing')
  return mode
}
export function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

export function AppProviders({
  children,
  queryClient,
  dependencies,
}: PropsWithChildren<{
  queryClient: QueryClient
  dependencies?: AppDependencies
}>) {
  const [services] = useState(() => dependencies ?? createAppDependencies())
  return (
    <BootstrapMode.Provider value={services.mode}>
      <AuthDependenciesProvider
        value={{ mode: services.mode, auth: services.auth }}
      >
        <CompetencyMapDependenciesProvider
          value={{ competencyMap: services.competencyMap }}
        >
          <DiagnosticDependenciesProvider
            value={{
              apiBase: services.apiBase,
              createDiagnosticIdentity: services.createDiagnosticIdentity,
              diagnosticStorage: services.diagnosticStorage,
              diagnostic: services.diagnostic,
            }}
          >
            <TrainerSessionDependenciesProvider
              value={{ trainerSessionSource: services.trainerSessionSource }}
            >
              <VoiceAnswerDependenciesProvider
                value={{
                  voice: services.voice,
                  assessment: services.assessment,
                }}
              >
                <Theme
                  className={styles.root}
                  accentColor="blue"
                  appearance="light"
                >
                  <QueryClientProvider client={queryClient}>
                    {children}
                  </QueryClientProvider>
                </Theme>
              </VoiceAnswerDependenciesProvider>
            </TrainerSessionDependenciesProvider>
          </DiagnosticDependenciesProvider>
        </CompetencyMapDependenciesProvider>
      </AuthDependenciesProvider>
    </BootstrapMode.Provider>
  )
}
