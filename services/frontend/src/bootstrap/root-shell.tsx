import { Button, Text } from '@radix-ui/themes'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, Outlet, useRouterState } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { useAuth } from '@/features/auth'
import { AppProviders } from './providers'
import styles from './root.module.scss'

export function RootShell({ queryClient }: { queryClient: QueryClient }) {
  return (
    <AppProviders queryClient={queryClient}>
      <div className={styles.app}>
        <SessionGate />
      </div>
    </AppProviders>
  )
}

function SessionGate() {
  const { t } = useTranslation()
  const { user, checkingSession, sessionError, retrySession, authenticate } =
    useAuth()
  const { path, renderedPath } = useRouterState({
    select: (state) => ({
      path: state.location.pathname,
      renderedPath: state.matches[state.matches.length - 1]?.pathname,
    }),
  })
  // The admin panel under /admin is a separate site with its own session; the
  // student site never links to it.
  const adminPortal = path === '/admin' || path.startsWith('/admin/')
  const entering = adminPortal && !checkingSession && !sessionError && !user
  // Opening the admin panel signs in to the shared teacher account at once;
  // a failed attempt waits for the retry button.
  useEffect(() => {
    if (entering && !authenticate.isPending && !authenticate.isError)
      authenticate.mutate({ mode: 'teacher' })
  }, [entering, authenticate])
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
  if (adminPortal) {
    if (!user)
      return (
        <main className={styles.screen}>
          {authenticate.isError ? (
            <div className={styles.sessionError}>
              <Text as="p" role="alert">
                {t('auth.adminEnterError')}
              </Text>
              <Button onClick={() => authenticate.mutate({ mode: 'teacher' })}>
                {t('trainer.retry')}
              </Button>
            </div>
          ) : (
            <Text role="status">{t('auth.adminEntering')}</Text>
          )}
        </main>
      )
    if (path === '/admin' || path === '/admin/')
      return <Navigate to="/admin/courses" replace />
  } else {
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
    if (path === '/login') return <Navigate to="/" replace />
  }
  if (renderedPath !== path) return loading
  return <Outlet />
}
