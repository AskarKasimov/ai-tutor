import { Badge, Flex, Text } from '@radix-ui/themes'
import { FileText } from 'lucide-react'
import type { ReactNode } from 'react'
import styles from './course-material-card.module.scss'

type MaterialStatusColor = 'amber' | 'blue' | 'green' | 'red'

export function CourseMaterialCard({
  name,
  status,
  statusColor,
  meta,
  actions,
  children,
  dropTarget = false,
}: {
  name: string
  status: string
  statusColor: MaterialStatusColor
  meta: ReactNode
  actions?: ReactNode
  children: ReactNode
  dropTarget?: boolean
}) {
  return (
    <article className={styles.card} data-drop-target={dropTarget || undefined}>
      <header className={styles.header}>
        <Flex gap="2" align="center" className={styles.title}>
          <FileText size={18} aria-hidden="true" />
          <Text weight="bold" size="3" className={styles.name}>
            {name}
          </Text>
          <Badge color={statusColor} size="1">
            {status}
          </Badge>
        </Flex>
        {actions && <div className={styles.actions}>{actions}</div>}
      </header>
      <div className={styles.meta}>{meta}</div>
      {children}
    </article>
  )
}
