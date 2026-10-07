import { Button, Heading, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { TrainerSession } from '../shared/domain'
import styles from './index.module.scss'

export function SessionSummary({
  session,
  onRestart,
  onReview,
  pending,
  error,
}: {
  session: TrainerSession
  onRestart: () => void
  onReview: (id: string) => void
  pending: boolean
  error: boolean
}) {
  const { t } = useTranslation()
  const total = session.answers.reduce(
    (sum, answer) => sum + answer.assessment.score,
    0,
  )
  return (
    <main className={styles.summary}>
      <Text as="p" className={styles.eyebrow}>
        {t('session.summary')}
      </Text>
      <Heading as="h1">{t('session.completed')}</Heading>
      <Text as="p" className={styles.summaryHelp}>
        {t('session.completedHelp')}
      </Text>
      <div className={styles.summaryScore}>
        <Text>{total} / 14</Text>
        <Text>{t('session.totalScore')}</Text>
      </div>
      <ol className={styles.summaryList}>
        {session.tasks.map((task, i) => {
          const answer = session.answers.find(
            (item) => item.assignmentId === task.assignmentId,
          )
          return (
            <li key={task.assignmentId}>
              <button
                onClick={() => onReview(task.assignmentId)}
                disabled={pending}
              >
                <span className={styles.summaryNumber}>
                  {String(i + 1).padStart(2, '0')}
                </span>
                <span className={styles.summaryQuestion}>
                  {t(task.questionKey)}
                </span>
                <span className={styles.summaryAnswerScore}>
                  {answer?.assessment.score} / 2
                </span>
              </button>
            </li>
          )
        })}
      </ol>
      {error && (
        <Text as="p" role="alert" className={styles.speechError}>
          {t('session.loadError')}
        </Text>
      )}
      <Button className={styles.primary} disabled={pending} onClick={onRestart}>
        {t(pending ? 'session.loading' : 'session.restart')}
      </Button>
    </main>
  )
}
