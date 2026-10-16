import { Text } from '@radix-ui/themes'
import { ChevronRight, GraduationCap } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AccountMenu } from '@/features/auth'
import styles from './session.module.scss'

// The progress bar under the header: a label on the left, whole percent on
// the right and a continuous bar.
export function TrainerProgress({
  label,
  percent,
}: {
  label: string
  percent: number
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.progress}>
      <div className={styles.progressText}>
        <Text>{label}</Text>
        <Text className={styles.progressPercent}>
          {t('session.percent', { value: percent })}
        </Text>
      </div>
      <div
        className={styles.progressTrack}
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        aria-valuetext={t('session.percent', { value: percent })}
      >
        <div className={styles.progressFill} style={{ width: `${percent}%` }} />
      </div>
    </div>
  )
}

export function TrainerShell({
  title,
  subject,
  crumb,
  onBack,
  navigator,
  progress,
  children,
}: {
  title: string
  subject: string
  crumb?: string
  onBack?: () => void
  navigator?: ReactNode
  progress?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div data-trainer-app className={styles.shell}>
      <div
        className={`${styles.frame} ${progress ? styles.frameWithProgress : ''}`}
      >
        <header className={styles.header}>
          {onBack ? (
            // The product logo returns to the student home.
            <button
              type="button"
              className={`${styles.brand} ${styles.brandLink}`}
              aria-label={t('home.back')}
              title={t('home.back')}
              onClick={onBack}
            >
              <span className={styles.logo} aria-hidden="true">
                <GraduationCap size={18} />
              </span>
              <Text>{t('trainer.title')}</Text>
            </button>
          ) : (
            <div className={styles.brand}>
              <span className={styles.logo} aria-hidden="true">
                <GraduationCap size={18} />
              </span>
              <Text>{t('trainer.title')}</Text>
            </div>
          )}
          {/* The course leads home; the current step is a non-clickable pill. */}
          <nav className={styles.crumbs} aria-label={title}>
            {onBack ? (
              <button
                type="button"
                className={styles.crumbLink}
                onClick={onBack}
              >
                {subject}
              </button>
            ) : (
              <span className={styles.crumbText}>{subject}</span>
            )}
            {crumb && (
              <>
                <ChevronRight
                  size={16}
                  className={styles.crumbDivider}
                  aria-hidden="true"
                />
                <strong className={styles.crumbCurrent} aria-current="page">
                  {crumb}
                </strong>
              </>
            )}
          </nav>
          <div className={styles.account}>
            <AccountMenu />
          </div>
        </header>
        {progress}
        <div className={`${styles.body} ${navigator ? '' : styles.bodyWide}`}>
          {navigator}
          <div className={styles.stage}>{children}</div>
        </div>
      </div>
    </div>
  )
}
