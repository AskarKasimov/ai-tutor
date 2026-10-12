import { AuthForm } from '@/features/auth'
import { Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import styles from './login.module.scss'

// The student site opens straight on sign-in; the admin panel is a separate
// site under /admin with its own session.
export function LoginScreen() {
  const { t } = useTranslation()
  return (
    <main className={styles.screen}>
      <div className={styles.content}>
        {/* The product promise greets the student before signing in. */}
        <Text as="p" className={styles.slogan}>
          {t('auth.slogan')}
        </Text>
        <AuthForm />
      </div>
    </main>
  )
}
