import { Text } from '@radix-ui/themes'
import { X } from 'lucide-react'
import { useEffect } from 'react'
import styles from './toast.module.scss'

type Props = {
  message: string
  closeLabel: string
  onClose: () => void
  // The toast hides itself after this many milliseconds; 0 keeps it.
  duration?: number
}

// A pop-up notice over the screen that does not take the focus: screen
// readers announce it, and it closes by itself or with its button.
export function Toast({
  message,
  closeLabel,
  onClose,
  duration = 8000,
}: Props) {
  useEffect(() => {
    if (!duration) return
    const timer = setTimeout(onClose, duration)
    return () => clearTimeout(timer)
  }, [duration, onClose])
  return (
    <div className={styles.toast} role="status" aria-live="polite">
      <Text as="p" size="2" className={styles.message}>
        {message}
      </Text>
      <button
        type="button"
        className={styles.close}
        aria-label={closeLabel}
        title={closeLabel}
        onClick={onClose}
      >
        <X size={16} aria-hidden="true" />
      </button>
    </div>
  )
}
