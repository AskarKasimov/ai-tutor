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
