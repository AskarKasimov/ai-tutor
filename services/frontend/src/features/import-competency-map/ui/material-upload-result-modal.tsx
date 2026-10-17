import { CircleAlert, CircleCheck, TriangleAlert, X } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import ReactModal from 'react-modal'
import styles from './material-upload-result-modal.module.scss'

type UploadNotification = {
  type: 'success' | 'error' | 'warning'
  message: string
}

export function MaterialUploadResultModal({
  notification,
  onClose,
}: {
  notification: UploadNotification | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const titleId = useId()
  const descriptionId = useId()
  const type = notification?.type ?? 'success'
  const Icon =
    type === 'success'
      ? CircleCheck
      : type === 'warning'
        ? TriangleAlert
        : CircleAlert
  const title = t(
    type === 'success'
      ? 'competencyMap.uploadResultSuccess'
      : type === 'warning'
        ? 'competencyMap.uploadResultWarning'
        : 'competencyMap.uploadResultError',
  )

  return (
    <ReactModal
      isOpen={notification !== null}
      appElement={
        document.getElementById('root') ??
        document.querySelector<HTMLElement>('[data-admin-screen]') ??
        undefined
      }
      onRequestClose={onClose}
      aria={{ labelledby: titleId, describedby: descriptionId }}
      className={styles.modal}
      overlayClassName={styles.overlay}
    >
      <button
        type="button"
        className={styles.close}
        onClick={onClose}
        aria-label={t('competencyMap.uploadResultClose')}
      >
        <X size={20} aria-hidden="true" />
      </button>
      <div className={styles.icon} data-variant={type}>
        <Icon size={32} strokeWidth={1.8} aria-hidden="true" />
      </div>
      <h2 id={titleId} className={styles.title}>
        {title}
      </h2>
      <p id={descriptionId} className={styles.message}>
        {notification?.message}
      </p>
      <button type="button" className={styles.confirm} onClick={onClose}>
        {t('competencyMap.uploadResultDismiss')}
      </button>
    </ReactModal>
  )
}
