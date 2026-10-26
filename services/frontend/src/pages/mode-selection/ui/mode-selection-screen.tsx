import { Button, Card, Flex, Heading, Text } from '@radix-ui/themes'
import { ArrowRight, BookOpen, Mic } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { AuthForm, AccountMenu, useAuth } from '@/features/auth'
import styles from './mode-selection.module.scss'

export function ModeSelectionScreen() {
  const { t } = useTranslation()
  const { user, checkingSession, sessionError, retrySession } = useAuth()
  if (checkingSession)
    return (
      <main className={styles.center}>
        <Text role="status">{t('auth.checkingSession')}</Text>
      </main>
    )
  if (sessionError)
    return (
      <main className={styles.center}>
        <Card className={styles.card}>
          <Text role="alert">{t('auth.networkError')}</Text>
          <Button onClick={() => void retrySession()}>
            {t('trainer.retry')}
          </Button>
        </Card>
      </main>
    )
  if (!user)
    return (
      <main className={styles.center}>
        <div className={styles.login}>
          <Text as="p" className={styles.brand}>
            AI-репетитор
          </Text>
          <AuthForm />
        </div>
      </main>
    )
  return (
    <>
      <header className={styles.header}>
        <Text className={styles.brand}>AI-репетитор</Text>
        <AccountMenu />
      </header>
      <main className={styles.center}>
        <section className={styles.choice} aria-labelledby="mode-title">
          <Heading id="mode-title" size="7">
            {t('modeSelection.title')}
          </Heading>
          <Text as="p" className={styles.lead}>
            {t('modeSelection.subtitle')}
          </Text>
          <Flex direction="column" gap="4">
            <Link to="/learn" className={styles.mode}>
              <span className={styles.learnIcon}>
                <BookOpen aria-hidden="true" />
              </span>
              <span>
                <strong>{t('modeSelection.learn')}</strong>
                <Text as="span">{t('modeSelection.learnDescription')}</Text>
              </span>
              <ArrowRight className={styles.arrow} aria-hidden="true" />
            </Link>
            <Link to="/exam" className={`${styles.mode} ${styles.exam}`}>
              <span className={styles.examIcon}>
                <Mic aria-hidden="true" />
              </span>
              <span>
                <strong>{t('modeSelection.exam')}</strong>
                <Text as="span">{t('modeSelection.examDescription')}</Text>
              </span>
              <ArrowRight className={styles.arrow} aria-hidden="true" />
            </Link>
          </Flex>
        </section>
      </main>
    </>
  )
}
