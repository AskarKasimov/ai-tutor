import type { PropsWithChildren } from 'react'
import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { Link } from '@tanstack/react-router'
import { useNavigate } from '@tanstack/react-router'
import { ShieldCheck } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { AccountMenu } from '@/features/auth'
import styles from './admin-course-detail-shell.module.scss'

type Direction = 'learn' | 'exam'

type Props = PropsWithChildren<{
  active: Direction
  subjectId: string
  subjectName?: string
  courseMissing?: boolean
  ready?: boolean
  title: string
  description: string
}>

export function AdminCourseDetailShell({
  active,
  subjectId,
  subjectName,
  courseMissing = false,
  ready,
  title,
  description,
  children,
}: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  useEffect(() => {
    if (courseMissing) void navigate({ to: '/', replace: true })
  }, [courseMissing, navigate])

  if (courseMissing) return null

  return (
    <div className={styles.page} data-admin-screen>
      <header className={styles.header}>
        <div className={styles.brand}>
          <ShieldCheck size={24} aria-hidden="true" />
          <Text weight="bold">{t('adminCourses.admin')}</Text>
        </div>
        <AccountMenu canLogout={false} />
      </header>
      <main className={styles.content}>
        <section className={styles.courseHeader} aria-labelledby="course-name">
          <div className={styles.courseIdentity}>
            <Text as="p" size="2" weight="medium" color="gray">
              {t('adminCourses.courseLabel')}
            </Text>
            <Heading as="h1" size="7" id="course-name">
              {subjectName ?? t('adminCourses.courseLoading')}
            </Heading>
          </div>
          <div className={styles.courseNavigation}>
            <Button asChild variant="soft">
              <Link to="/admin/courses" activeOptions={{ exact: true }}>
                {t('adminCourses.backToCourses')}
              </Link>
            </Button>
            <nav
              className={styles.directionNav}
              aria-label={t('directionNav.adminLabel')}
            >
              <Link
                to="/admin/courses/$subjectId"
                params={{ subjectId }}
                search={{ direction: 'learn' }}
                activeOptions={{ exact: true, includeSearch: true }}
                className={styles.directionLink}
                aria-current={active === 'learn' ? 'page' : undefined}
                data-selected={active === 'learn' ? 'true' : undefined}
              >
                {t('directionNav.learnMaterials')}
              </Link>
              <Link
                to="/admin/courses/$subjectId"
                params={{ subjectId }}
                search={{ direction: 'exam' }}
                activeOptions={{
                  exact: true,
                  includeSearch: true,
                  explicitUndefined: true,
                }}
                className={styles.directionLink}
                aria-current={active === 'exam' ? 'page' : undefined}
                data-selected={active === 'exam' ? 'true' : undefined}
              >
                {t('directionNav.examMaterials')}
              </Link>
            </nav>
          </div>
        </section>
        <section className={styles.directionHeader}>
          <div className={styles.directionIntro}>
            <Heading as="h2" size="6">
              {title}
            </Heading>
            <Text as="p" color="gray">
              {description}
            </Text>
          </div>
          <Badge color={ready ? 'green' : 'gray'}>
            {ready
              ? t(
                  active === 'learn'
                    ? 'adminCourses.learnReady'
                    : 'adminCourses.examReady',
                )
              : t(
                  active === 'learn'
                    ? 'adminCourses.learnPreparing'
                    : 'adminCourses.examPreparing',
                )}
          </Badge>
        </section>
        {children}
      </main>
    </div>
  )
}
