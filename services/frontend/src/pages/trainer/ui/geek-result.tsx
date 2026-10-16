import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { TopicStatus } from '@/entities/diagnostic-session'
import { useDiagnosticResultQuery } from '@/features/diagnostic-session'
import { TrainerShell } from './trainer-shared'
import styles from './geek.module.scss'

const statusTone: Record<TopicStatus, 'green' | 'blue' | 'red' | 'gray'> = {
  independent: 'green',
  assisted: 'blue',
  gap: 'red',
  insufficient: 'gray',
  credited_by_parent: 'green',
  unchecked: 'gray',
}

// The outcome of a completed diagnostic: the preliminary course grade, the
// gaps to work on and every topic with its status.
export function GeekResult({
  userId,
  sessionId,
  subjectName,
  onBack,
  onAgain,
  starting = false,
  startError = '',
}: {
  userId: string
  sessionId: string
  subjectName: string
  onBack: () => void
  onAgain: () => void
  // The state of "start again", which runs from this screen.
  starting?: boolean
  startError?: string
}) {
  const { t } = useTranslation()
  const query = useDiagnosticResultQuery(userId, sessionId)
  const result = query.data
  return (
    <TrainerShell
      title={t('geek.resultTitle')}
      subject={subjectName}
      crumb={t('geek.resultTitle')}
      onBack={onBack}
    >
      <main className={styles.result}>
        <Heading as="h1" size="7">
          {t('geek.resultTitle')}
        </Heading>
        {query.isPending ? (
          <Text role="status">{t('geek.resultLoading')}</Text>
        ) : query.isError || !result ? (
          <div className={styles.error} role="alert">
            <Text as="p">{t('geek.resultError')}</Text>
            <Button variant="soft" onClick={() => void query.refetch()}>
              {t('trainer.retry')}
            </Button>
          </div>
        ) : (
          <>
            <section
              className={styles.grade}
              data-grade={result.grade ?? 'none'}
              aria-labelledby="grade-title"
            >
              <Text as="p" id="grade-title" className={styles.gradeLabel}>
                {t('geek.gradeLabel')}
              </Text>
              <Text as="p" className={styles.gradeValue}>
                {result.grade ?? '—'}
              </Text>
              <Text as="p" color="gray">
                {result.scorePercent === null
                  ? t('geek.gradeNoneHint')
                  : t('geek.score', { percent: result.scorePercent })}
              </Text>
              <Text as="p" size="2" color="gray">
                {t('geek.gradeNote')}
              </Text>
            </section>
            <section aria-labelledby="gaps-title" className={styles.section}>
              <Heading as="h2" size="5" id="gaps-title">
                {t('geek.gapsTitle')}
              </Heading>
              {result.gaps.length === 0 ? (
                <Text as="p">{t('geek.gapsEmpty')}</Text>
              ) : (
                <>
                  <Text as="p" color="gray" size="2">
                    {t('geek.gapsHint')}
                  </Text>
                  <ul className={styles.gaps}>
                    {result.gaps.map((gap) => (
                      <li key={gap.nodeId}>{gap.title}</li>
                    ))}
                  </ul>
                </>
              )}
            </section>
            <section aria-labelledby="topics-title" className={styles.section}>
              <Heading as="h2" size="5" id="topics-title">
                {t('geek.topicsTitle')}
              </Heading>
              <ul className={styles.topics}>
                {result.topics.map((topic) => (
                  <li key={topic.nodeId} className={styles.topic}>
                    <span className={styles.topicTitle}>
                      <Text weight={topic.topLevel ? 'bold' : 'regular'}>
                        {topic.title}
                      </Text>
                      {topic.topLevel && (
                        <Text size="1" color="gray">
                          {t('geek.topLevel')}
                        </Text>
                      )}
                    </span>
                    <Badge color={statusTone[topic.status]}>
                      {t(`geek.status.${topic.status}`)}
                    </Badge>
                    <Text className={styles.topicGrade}>
                      {topic.grade ?? '—'}
                    </Text>
                  </li>
                ))}
              </ul>
            </section>
          </>
        )}
        {startError && (
          <Text as="p" role="alert" color="red">
            {startError}
          </Text>
        )}
        <div className={styles.resultActions}>
          <Button size="3" disabled={starting} onClick={onAgain}>
            {t(starting ? 'home.starting' : 'geek.again')}
          </Button>
          <Button size="3" variant="soft" onClick={onBack}>
            {t('home.back')}
          </Button>
        </div>
      </main>
    </TrainerShell>
  )
}
