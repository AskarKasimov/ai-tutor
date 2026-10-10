import { DropdownMenu } from '@radix-ui/themes'
import { ChevronDown, LogOut } from 'lucide-react'
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
  // Accounts created before the name became required fall back to email.
  const name = user?.display_name || user?.email || ''
  return (
    <div className={styles.account}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>
          <button
            type="button"
            className={styles.profile}
            aria-label={`${t('auth.accountMenu')}: ${name}`}
          >
            <span className={styles.avatar} aria-hidden="true">
              {initials(name)}
            </span>
            <span className={styles.user}>{name}</span>
            <ChevronDown size={16} aria-hidden="true" />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Content align="end" sideOffset={6}>
          <DropdownMenu.Item
            color="red"
            disabled={logout.isPending}
            onSelect={() => logout.mutate()}
          >
            <LogOut size={16} aria-hidden="true" />
            {t('auth.logout')}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      {logout.isError && (
        <span role="alert" className={styles.error}>
          {t('auth.networkError')}
        </span>
      )}
    </div>
  )
}
