import { Theme } from '@radix-ui/themes'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import type { PropsWithChildren } from 'react'
import { AuthDependenciesProvider } from '@/features/auth'
import { CompetencyMapDependenciesProvider } from '@/features/import-competency-map'
import { DiagnosticDependenciesProvider } from '@/features/diagnostic-session'
import { SubjectSelectionDependenciesProvider } from '@/features/subject-selection'
import { LearningPlanDependenciesProvider } from '@/features/learning-plan'
import { createAppDependencies } from './dependencies'
import type { AppDependencies } from './dependencies'
import '@/shared/i18n'
import styles from './providers.module.scss'

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
    <AuthDependenciesProvider value={{ auth: services.auth }}>
      <CompetencyMapDependenciesProvider
        value={{ competencyMap: services.competencyMap }}
      >
        <SubjectSelectionDependenciesProvider
          value={{ subjects: services.subjects }}
        >
          <DiagnosticDependenciesProvider
            value={{
              apiBase: services.apiBase,
              createDiagnosticIdentity: services.createDiagnosticIdentity,
              diagnosticStorage: services.diagnosticStorage,
              diagnostic: services.diagnostic,
            }}
          >
            <LearningPlanDependenciesProvider
              value={{ learningPlan: services.learningPlan }}
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
            </LearningPlanDependenciesProvider>
          </DiagnosticDependenciesProvider>
        </SubjectSelectionDependenciesProvider>
      </CompetencyMapDependenciesProvider>
    </AuthDependenciesProvider>
  )
}
