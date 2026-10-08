import { Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { useBootstrapMode } from '@/bootstrap/providers'
import styles from './dev-banner.module.scss'

export function DevBanner() {
  const mode = useBootstrapMode()
  const { t } = useTranslation()
  return mode === 'demo' ? (
    <div
      className={styles.devBanner}
      role="note"
      aria-label={t('mockApi.mode')}
    >
      <Text className={styles.devBannerLabel}>{t('mockApi.mode')}</Text>
    </div>
  ) : null
}
