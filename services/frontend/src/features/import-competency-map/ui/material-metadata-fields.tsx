import { Select, Text, TextField } from '@radix-ui/themes'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { CourseMaterialKindOption } from '@/entities/competency-map'
import styles from './material-metadata-fields.module.scss'

export function MaterialMetadataFields({
  week,
  kind,
  kinds,
  disabled,
  onWeekChange,
  onKindChange,
  action,
}: {
  week: string
  kind: string
  kinds: CourseMaterialKindOption[]
  disabled: boolean
  onWeekChange: (week: string) => void
  onKindChange: (kind: string) => void
  action?: ReactNode
}) {
  const { t } = useTranslation()

  return (
    <div className={styles.fields}>
      <label className={styles.weekField}>
        <Text as="span" size="2">
          {t('courseGraph.weekLabel')}
        </Text>
        <TextField.Root
          className={styles.control}
          size="3"
          type="number"
          min="1"
          step="1"
          value={week}
          disabled={disabled}
          onChange={(event) => onWeekChange(event.target.value)}
          aria-label={t('courseGraph.weekLabel')}
        />
      </label>
      <label className={styles.kindField}>
        <Text as="span" size="2">
          {t('courseGraph.kindLabel')}
        </Text>
        <Select.Root
          size="3"
          value={kind}
          disabled={disabled}
          onValueChange={onKindChange}
        >
          <Select.Trigger
            className={styles.control}
            aria-label={t('courseGraph.kindLabel')}
          />
          <Select.Content>
            {kinds.map((option) => (
              <Select.Item key={option.code} value={option.code}>
                {option.label}
              </Select.Item>
            ))}
          </Select.Content>
        </Select.Root>
      </label>
      {action && <div className={styles.action}>{action}</div>}
    </div>
  )
}
