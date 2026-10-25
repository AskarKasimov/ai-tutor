import { Badge, Button, Flex, Heading, Text, TextField } from '@radix-ui/themes'
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BookOpen, Plus, ShieldCheck } from 'lucide-react'
import { AccountMenu, useAuth } from '@/features/auth'
import {
  useCreateSubjectMutation,
  useSubjectsQuery,
} from '@/features/subject-selection'
import styles from './admin-courses.module.scss'
import { DirectionNav } from '@/widgets/direction-nav'

export function AdminCoursesScreen() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [showForm, setShowForm] = useState(false)
  const [courseName, setCourseName] = useState('')
  const userId = user?.id ?? ''
  const courses = useSubjectsQuery(userId)
  const createCourse = useCreateSubjectMutation(userId)

  if (user?.role !== 'admin')
    return (
      <main className={styles.denied}>
        <Heading as="h1">{t('adminCourses.title')}</Heading>
        <Text as="p" role="alert">
          {t('competencyMap.forbidden')}
        </Text>
      </main>
    )

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <ShieldCheck size={24} aria-hidden="true" />
          <Text weight="bold">{t('competencyMap.admin')}</Text>
        </div>
        <AccountMenu canLogout={false} />
      </header>
      <main className={styles.content}>
        <DirectionNav audience="admin" active="exam" />
        <div className={styles.intro}>
          <div>
            <Heading as="h1" size="8">
              {t('adminCourses.title')}
            </Heading>
            <Text as="p" color="gray">
              {t('adminCourses.subtitle')}
            </Text>
          </div>
          <Button size="3" onClick={() => setShowForm((open) => !open)}>
            <Plus size={18} aria-hidden="true" />
            {t('adminCourses.newCourse')}
          </Button>
        </div>

        {showForm && (
          <section
            className={styles.card}
            aria-label={t('adminCourses.newCourse')}
          >
            <TextField.Root
              size="3"
              aria-label={t('adminCourses.courseName')}
              placeholder={t('competencyMap.subjectPlaceholder')}
              value={courseName}
              disabled={createCourse.isPending}
              onChange={(event) => setCourseName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') submitCourse()
              }}
            />
            {createCourse.isError && (
              <Text as="p" role="alert" color="red" size="2">
                {t('adminCourses.createError')}
              </Text>
            )}
            <Flex gap="3" wrap="wrap">
              <Button
                disabled={!courseName.trim() || createCourse.isPending}
                onClick={submitCourse}
              >
                {t('adminCourses.create')}
              </Button>
              <Button
                variant="soft"
                color="gray"
                onClick={() => setShowForm(false)}
              >
                {t('competencyMap.cancel')}
              </Button>
            </Flex>
          </section>
        )}

        {courses.isPending ? (
          <Text as="p" role="status">
            {t('adminCourses.loading')}
          </Text>
        ) : courses.isError ? (
          <section className={styles.card}>
            <Text as="p" role="alert" color="red">
              {t('adminCourses.loadError')}
            </Text>
            <Button variant="soft" onClick={() => void courses.refetch()}>
              {t('trainer.retry')}
            </Button>
          </section>
        ) : courses.data.length === 0 ? (
          <section className={styles.card}>
            <Text color="gray">{t('adminCourses.empty')}</Text>
          </section>
        ) : (
          <section
            className={styles.list}
            aria-label={t('adminCourses.coursesTitle')}
          >
            {courses.data.map((course) => (
              <Link
                className={styles.course}
                key={course.id}
                to="/admin/competency-map/$subjectId"
                params={{ subjectId: course.id }}
              >
                <div className={styles.courseName}>
                  <BookOpen size={20} aria-hidden="true" />
                  <Heading as="h2" size="4">
                    {course.name}
                  </Heading>
                </div>
                <Badge color={course.ready ? 'green' : 'gray'}>
                  {t(
                    course.ready
                      ? 'adminCourses.ready'
                      : 'adminCourses.preparing',
                  )}
                </Badge>
              </Link>
            ))}
          </section>
        )}
      </main>
    </div>
  )

  function submitCourse() {
    const name = courseName.trim()
    if (!name) return
    createCourse.mutate(name, {
      onSuccess: () => {
        setCourseName('')
        setShowForm(false)
      },
    })
  }
}
