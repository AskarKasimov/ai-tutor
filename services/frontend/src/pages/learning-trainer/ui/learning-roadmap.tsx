import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { BookOpen, ChevronDown, Flag, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useCompetencyMapOutlineQuery } from '@/features/import-learning-competency-map'
import styles from './learning-roadmap.module.scss'

export function LearningRoadmap({
  userId,
  subjectId,
}: {
  userId: string
  subjectId: string
}) {
  const { t } = useTranslation()
  const query = useCompetencyMapOutlineQuery(userId, subjectId)
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
          <ol
            className={styles.path}
            aria-label={t('learningRoadmap.pathLabel')}
          >
            {competencies.map((competency, index) => (
              <li key={competency.id} className={styles.step}>
                <span className={styles.number} aria-hidden="true">
                  {String(index + 1).padStart(2, '0')}
                </span>
                <details className={styles.competency} open={index === 0}>
                  <summary className={styles.summary}>
                    <span className={styles.summaryText}>
                      <span className={styles.name}>{competency.name}</span>
                      <Text as="span" size="2" color="gray">
                        {t('learningRoadmap.topicCount', {
                          count: competency.constituents.length,
                        })}
                      </Text>
                    </span>
                    <ChevronDown
                      size={20}
                      className={styles.chevron}
                      aria-hidden="true"
                    />
                  </summary>
                  <div className={styles.topics}>
                    {competency.constituents.map((topic) => (
                      <section key={topic.id} className={styles.topic}>
                        <Heading as="h3" size="3" className={styles.topicTitle}>
                          <BookOpen size={17} aria-hidden="true" />
                          {topic.name}
                        </Heading>
                        <ul className={styles.outcomes}>
                          {topic.outcomes.map((outcome) => (
                            <li key={outcome.id}>{outcome.name}</li>
                          ))}
                        </ul>
                      </section>
                    ))}
                  </div>
                </details>
              </li>
            ))}
          </ol>
          <div className={styles.destination}>
            <Flag size={20} aria-hidden="true" />
            <Text size="2">{t('learningRoadmap.next')}</Text>
          </div>
        </>
      )}
    </section>
  )
}
