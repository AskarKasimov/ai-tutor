import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import styles from './role-selection.module.scss'

export function RoleSelectionScreen() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { authenticate } = useAuth()
  return (
    <main className={styles.screen}>
      <Card className={styles.card}>
        <Heading as="h1">{t('auth.chooseRole')}</Heading>
        <Text as="p">{t('auth.roleSubtitle')}</Text>
        <div className={styles.actions}>
          <Button
            size="3"
            disabled={authenticate.isPending}
            onClick={() => authenticate.mutate({ mode: 'teacher' })}
          >
            {t(
              authenticate.isPending
                ? 'auth.teacherEntering'
                : 'auth.teacherLogin',
            )}
          </Button>
          <Button
            size="3"
            variant="soft"
            disabled={authenticate.isPending}
            onClick={() => void navigate({ to: '/login' })}
          >
            {t('auth.studentLogin')}
          </Button>
        </div>
        {authenticate.isError && (
          <Text as="p" role="alert">
            {authenticate.error.message}
          </Text>
        )}
      </Card>
    </main>
  )
}
