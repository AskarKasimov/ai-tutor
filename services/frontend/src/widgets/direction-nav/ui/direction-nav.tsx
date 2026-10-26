import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import styles from './direction-nav.module.scss'

type Direction = 'learn' | 'exam'
type DirectionNavProps =
  | { active: Direction; audience: 'admin'; subjectId: string }
  | { active: Direction; audience: 'student'; subjectId?: string }

export function DirectionNav({
  active,
  audience,
  subjectId,
}: DirectionNavProps) {
  const { t } = useTranslation()
  return (
    <nav
      className={styles.nav}
      aria-label={t(
        audience === 'admin'
          ? 'directionNav.adminLabel'
          : 'directionNav.studentLabel',
      )}
    >
      {audience === 'admin' ? (
        <>
          <Link
            to="/admin/courses/$subjectId"
            params={{ subjectId }}
            search={{ direction: 'learn' }}
            activeOptions={{ exact: true, includeSearch: true }}
            className={styles.link}
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
            className={styles.link}
            aria-current={active === 'exam' ? 'page' : undefined}
            data-selected={active === 'exam' ? 'true' : undefined}
          >
            {t('directionNav.examMaterials')}
          </Link>
        </>
      ) : (
        <>
          <Link
            to="/learn"
            search={subjectId ? { subject_id: subjectId } : {}}
            activeOptions={{ exact: true, includeSearch: true }}
            className={styles.link}
            aria-current={active === 'learn' ? 'page' : undefined}
            data-selected={active === 'learn' ? 'true' : undefined}
          >
            {t('directionNav.learn')}
          </Link>
          <Link
            to="/exam"
            search={subjectId ? { subject_id: subjectId } : {}}
            activeOptions={{ exact: true, includeSearch: true }}
            className={styles.link}
            aria-current={active === 'exam' ? 'page' : undefined}
            data-selected={active === 'exam' ? 'true' : undefined}
          >
            {t('directionNav.exam')}
          </Link>
        </>
      )}
    </nav>
  )
}
