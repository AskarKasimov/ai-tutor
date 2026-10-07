import { Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { isMockApi } from '../data/api-fetch'
import styles from './index.module.scss'

export function DevBanner() {
  const { t } = useTranslation()
  return isMockApi() ? (
    <div
      className={styles.devBanner}
      role="note"
      aria-label={t('mockApi.mode')}
    >
      <Text className={styles.devBannerLabel}>{t('mockApi.mode')}</Text>
    </div>
  ) : null
}
