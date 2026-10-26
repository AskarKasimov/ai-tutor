import { Button, Select, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import styles from './student-course-shell.module.scss'

export type StudentCourseOption = { id: string; name: string }

export function StudentCourseShell({
  courseName,
  courses = [],
  selectedCourseId,
  onCourseChange,
}: {
  courseName?: string
  courses?: StudentCourseOption[]
  selectedCourseId?: string
  onCourseChange?: (subjectId: string) => void
}) {
  const { t } = useTranslation()
  const canSelectCourse = courses.length > 1 && !!onCourseChange

  return (
    <section className={styles.shell} aria-label={t('studentShell.course')}>
      <div className={styles.course}>
        {canSelectCourse ? (
          <Select.Root
            value={selectedCourseId ?? ''}
            onValueChange={onCourseChange}
          >
            <Select.Trigger
              aria-label={t('studentShell.selectCourse')}
              variant="soft"
              className={styles.select}
              placeholder={t('studentShell.selectCourse')}
            />
            <Select.Content position="popper">
              {courses.map((subject) => (
                <Select.Item key={subject.id} value={subject.id}>
                  {subject.name}
                </Select.Item>
              ))}
            </Select.Content>
          </Select.Root>
        ) : (
          <Text as="div" size="4" weight="bold" className={styles.courseName}>
            {courseName ?? t('studentShell.selectCourse')}
          </Text>
        )}
      </div>
      <Button asChild variant="soft" className={styles.modeLink}>
        <a href="/">{t('studentShell.chooseMode')}</a>
      </Button>
    </section>
  )
}
