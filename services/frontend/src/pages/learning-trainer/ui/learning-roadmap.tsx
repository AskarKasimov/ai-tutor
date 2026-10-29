import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { Flag, Route } from 'lucide-react'
import { lazy, Suspense, useCallback } from 'react'
import type { CompetencyMapOutline } from '@/entities/learning-competency-map'
import { useTranslation } from 'react-i18next'
import type { useCompetencyMapOutlineQuery } from '@/features/import-learning-competency-map'
import styles from './learning-roadmap.module.scss'

const DiagnosticRoadmapCanvas = lazy(
  () => import('./diagnostic-roadmap-canvas'),
)

export function LearningRoadmap({
  query,
  subjectId,
  subjectName,
  onReady,
}: {
  query: ReturnType<typeof useCompetencyMapOutlineQuery>
  subjectId: string
  subjectName?: string
  onReady?: (
    subjectId: string,
    outline: CompetencyMapOutline,
    ready: boolean,
  ) => void
}) {
  const { t } = useTranslation()
  const competencies = query.data?.competencies ?? []
  const reportReady = useCallback(
    (ready: boolean) => {
      if (query.data) onReady?.(subjectId, query.data, ready)
    },
    [onReady, subjectId, query.data],
  )

  return (
    <section
      className={styles.roadmap}
      aria-labelledby="learning-roadmap-title"
    >
      <div className={styles.header}>
        <span className={styles.icon} aria-hidden="true">
          <Route size={24} />
        </span>
        <div>
          <Heading as="h2" size="5" id="learning-roadmap-title">
            {t('learningRoadmap.title')}
          </Heading>
          <Text as="p" size="2" color="gray">
            {t('learningRoadmap.intro')}
          </Text>
        </div>
        <Badge variant="soft" size="2">
          {t('learningRoadmap.preview')}
        </Badge>
      </div>
      {query.isPending || query.fetchStatus !== 'idle' ? (
        <Text role="status">{t('learningRoadmap.loading')}</Text>
      ) : query.isError ? (
        <div className={styles.notice}>
          <Text role="alert">{t('learningRoadmap.error')}</Text>
          <Button variant="soft" onClick={() => void query.refetch()}>
            {t('learningTrainer.retry')}
          </Button>
        </div>
      ) : competencies.length === 0 ? (
        <Text as="p" color="gray">
          {t('learningRoadmap.empty')}
        </Text>
      ) : (
        <>
          <Suspense
            fallback={<Text role="status">{t('learningRoadmap.loading')}</Text>}
          >
            {query.data && (
              <DiagnosticRoadmapCanvas
                key={subjectId}
                outline={query.data}
                onReady={reportReady}
                subjectTitle={subjectName || subjectId}
              />
            )}
          </Suspense>
          <div className={styles.destination}>
            <Flag size={20} aria-hidden="true" />
            <Text size="2">{t('learningRoadmap.next')}</Text>
          </div>
        </>
      )}
    </section>
  )
}
