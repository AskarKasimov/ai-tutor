import { Button, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { Navigate, Outlet, useRouterState } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { useAuth } from '@/features/auth'
import { AppProviders } from './providers'
import styles from './root.module.scss'
import { DevBanner } from './ui/dev-banner'

export function RootShell({ queryClient }: { queryClient: QueryClient }) {
  return (
    <AppProviders queryClient={queryClient}>
      <div className={styles.app}>
        <DevBanner />
        <SessionGate />
      </div>
    </AppProviders>
  )
}

function SessionGate() {
  const { t } = useTranslation()
  const { user, checkingSession, sessionError, retrySession } = useAuth()
  const { path, renderedPath } = useRouterState({
    select: (state) => ({
      path: state.location.pathname,
      renderedPath: state.matches[state.matches.length - 1]?.pathname,
    }),
  })
  const loading = (
    <main className={styles.screen}>
      <Text role="status">{t('auth.checkingSession')}</Text>
    </main>
  )
  if (checkingSession) return loading
  if (sessionError)
    return (
      <main className={styles.screen}>
        <div className={styles.sessionError}>
          <Text as="p" role="alert">
            {t('auth.networkError')}
          </Text>
          <Button onClick={() => void retrySession()}>
            {t('trainer.retry')}
          </Button>
        </div>
      </main>
    )
  if (!user)
    return path === '/login' ? (
      renderedPath === path ? (
        <Outlet />
      ) : (
        loading
      )
    ) : (
      <Navigate to="/login" replace />
    )
  const home = user.role === 'admin' ? '/admin/competency-map' : '/'
  if (path === '/login' || (path === '/' && user.role === 'admin'))
    return <Navigate to={home} replace />
  if (renderedPath !== path) return loading
  return <Outlet />
}
