import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import styles from './direction-nav.module.scss'

type Direction = 'learn' | 'exam'

export function DirectionNav({
  active,
  audience,
  subjectId,
}: {
  active: Direction
  audience: 'admin' | 'student'
  subjectId?: string
}) {
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
            to={
              subjectId
                ? '/admin/learn/competency-map/$subjectId'
                : '/admin/learn/competency-map'
            }
            params={subjectId ? { subjectId } : undefined}
            className={styles.link}
            aria-current={active === 'learn' ? 'page' : undefined}
          >
            {t('directionNav.learnMaterials')}
          </Link>
          <Link
            to={
              subjectId ? '/admin/competency-map/$subjectId' : '/admin/courses'
            }
            params={subjectId ? { subjectId } : undefined}
            className={styles.link}
            aria-current={active === 'exam' ? 'page' : undefined}
          >
            {t('directionNav.examMaterials')}
          </Link>
        </>
      ) : (
        <>
          <Link
            to="/learn"
            search={subjectId ? { subject_id: subjectId } : {}}
            className={styles.link}
            aria-current={active === 'learn' ? 'page' : undefined}
          >
            {t('directionNav.learn')}
          </Link>
          <Link
            to="/exam"
            search={subjectId ? { subject_id: subjectId } : {}}
            className={styles.link}
            aria-current={active === 'exam' ? 'page' : undefined}
          >
            {t('directionNav.exam')}
          </Link>
        </>
      )}
    </nav>
  )
}
