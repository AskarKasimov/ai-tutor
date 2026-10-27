import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { Flag, Route } from 'lucide-react'
import { lazy, Suspense, useMemo } from 'react'
import { buildDiagnosticGraph } from '../model/diagnostic-graph'
import { useTranslation } from 'react-i18next'
import { useCompetencyMapOutlineQuery } from '@/features/import-learning-competency-map'
import styles from './learning-roadmap.module.scss'

const CourseGraphCanvas = lazy(() => import('@/widgets/course-graph'))
const materialNames = new Map<string, string>()

export function LearningRoadmap({
  userId,
  subjectId,
}: {
  userId: string
  subjectId: string
}) {
  const { t } = useTranslation()
  const query = useCompetencyMapOutlineQuery(userId, subjectId)
  const graph = useMemo(
    () => (query.data ? buildDiagnosticGraph(query.data) : undefined),
    [query.data],
  )
  const competencies = query.data?.competencies ?? []
  const topics = competencies.flatMap((item) => item.constituents)
  const outcomes = topics.flatMap((item) => item.outcomes)

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
      {query.isPending ? (
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
          <Text as="p" size="2" className={styles.counts}>
            {t('learningRoadmap.counts', {
              competencies: competencies.length,
              topics: topics.length,
              outcomes: outcomes.length,
            })}
          </Text>
          <Suspense
            fallback={<Text role="status">{t('learningRoadmap.loading')}</Text>}
          >
            {graph && (
              <CourseGraphCanvas
                key={subjectId}
                graph={graph}
                materialNames={materialNames}
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
