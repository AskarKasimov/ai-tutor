import { Text } from '@radix-ui/themes'
import type { ComponentProps, ReactNode } from 'react'
import styles from './result-banner.module.scss'

// A bright card with the next step of the result screen.
export function ResultBanner({
  title,
  text,
  children,
}: {
  title: string
  text?: string
  children: ReactNode
}) {
  return (
    <div className={styles.banner}>
      <div className={styles.bannerCopy}>
        <Text as="p" className={styles.bannerTitle}>
          {title}
        </Text>
        {text && (
          <Text as="p" className={styles.bannerText}>
            {text}
          </Text>
        )}
      </div>
      <div className={styles.bannerActions}>{children}</div>
    </div>
  )
}

export function BannerPrimaryButton({
  icon,
  children,
  ...props
}: ComponentProps<'button'> & { icon: ReactNode }) {
  return (
    <button type="button" className={styles.primary} {...props}>
      <span className={styles.primaryIcon} aria-hidden="true">
        {icon}
      </span>
      {children}
    </button>
  )
}

export function BannerSecondaryButton(props: ComponentProps<'button'>) {
  return <button type="button" className={styles.secondary} {...props} />
}
