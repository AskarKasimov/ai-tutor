import { useTranslation } from 'react-i18next'
import { useAuth } from '../model/use-auth'
import styles from './auth-modal.module.scss'

export function AccountMenu() {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  return (
    <div className={styles.account}>
      <span className={styles.user}>{user?.display_name || user?.email}</span>
      <button
        className={styles.trigger}
        disabled={logout.isPending}
        onClick={() => logout.mutate()}
      >
        {t('auth.logout')}
      </button>
      {logout.isError && (
        <span role="alert" className={styles.error}>
          {t('auth.networkError')}
        </span>
      )}
    </div>
  )
}
