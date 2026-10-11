import { AuthForm } from '@/features/auth'
import { Button, Text } from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import styles from './login.module.scss'

export function LoginScreen() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <main className={styles.screen}>
      <div className={styles.content}>
        {/* The product promise greets the student before signing in. */}
        <Text as="p" className={styles.slogan}>
          {t('auth.slogan')}
        </Text>
        <div className={styles.banner}>
          <Text>{t('auth.studentBanner')}</Text>
          <Button
            variant="soft"
            onClick={() => void navigate({ to: '/welcome' })}
          >
            {t('auth.backToRoles')}
          </Button>
        </div>
        <AuthForm />
      </div>
    </main>
  )
}
