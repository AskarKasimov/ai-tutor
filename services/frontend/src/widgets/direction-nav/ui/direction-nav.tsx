import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import styles from './direction-nav.module.scss'

type Direction = 'learn' | 'exam'

export function DirectionNav({
  active,
  audience,
}: {
  active: Direction
  audience: 'admin' | 'student'
}) {
  const { t } = useTranslation()
  const items =
    audience === 'admin'
      ? [
          {
            direction: 'learn' as const,
            label: t('directionNav.learnMaterials'),
            to: '/admin/learn/competency-map' as const,
          },
          {
            direction: 'exam' as const,
            label: t('directionNav.examMaterials'),
            to: '/admin/courses' as const,
          },
        ]
      : [
          {
            direction: 'learn' as const,
            label: t('directionNav.learn'),
            to: '/learn' as const,
          },
          {
            direction: 'exam' as const,
            label: t('directionNav.exam'),
            to: '/exam' as const,
          },
        ]

  return (
    <nav
      className={styles.nav}
      aria-label={t(
        audience === 'admin'
          ? 'directionNav.adminLabel'
          : 'directionNav.studentLabel',
      )}
    >
      {items.map((item) => (
        <Link
          key={item.direction}
          to={item.to}
          className={styles.link}
          aria-current={item.direction === active ? 'page' : undefined}
        >
          {item.label}
        </Link>
      ))}
    </nav>
  )
}
