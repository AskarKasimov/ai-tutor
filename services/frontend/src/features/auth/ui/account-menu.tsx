import { LogOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../model/use-auth'
import styles from './auth-modal.module.scss'

function initials(name: string) {
  const words = name
    .replace(/@.*$/, '')
    .split(/[\s._-]+/)
    .filter(Boolean)
  return (
    words
      .slice(0, 2)
      .map((word) => word[0])
      .join('')
      .toUpperCase() || '?'
  )
}

export function AccountMenu() {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  const name = user?.display_name || user?.email || ''
  return (
    <div className={styles.account}>
      <span className={styles.avatar} aria-hidden="true">
        {initials(name)}
      </span>
      <span className={styles.user}>{name}</span>
      <button
        className={styles.trigger}
        disabled={logout.isPending}
        onClick={() => logout.mutate()}
      >
        <LogOut size={16} aria-hidden="true" />
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
