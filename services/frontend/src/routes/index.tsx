import { Heading, Text } from '@radix-ui/themes'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import styles from './index.module.scss'

export const Route = createFileRoute('/')({ component: HomePage })

function HomePage() {
  const { t } = useTranslation()

  return (
    <main className={styles.page}>
      <Heading as="h1">{t('routes.home.title')}</Heading>
      <Text as="p" color="gray">{t('routes.home.description')}</Text>
    </main>
  )
}
