import { Theme } from '@radix-ui/themes'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'

import '../i18n/i18n'
import styles from './providers.module.scss'

export function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

export function AppProviders({ children, queryClient }: PropsWithChildren<{ queryClient: QueryClient }>) {
  return (
    <Theme className={styles.root}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </Theme>
  )
}
