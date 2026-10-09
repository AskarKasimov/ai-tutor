import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { TrainingAttempt, TrainingProgress } from '@/entities/training'
import { useTrainingHistoryQuery } from '@/features/training'
import styles from './training.module.scss'

export function TrainingHistory({
  userId,
  progress,
}: {
  userId: string
  progress: TrainingProgress
}) {
  const { t } = useTranslation()
  const history = useTrainingHistoryQuery(userId, progress.session_id)
  const items = history.data?.pages.flatMap((page) => page.items) ?? []
  return (
    <section aria-labelledby="training-history-title">
      <Heading id="training-history-title" as="h2">
        {t('training.history')}
      </Heading>
      {history.isPending ? (
        <Text role="status">{t('training.historyLoading')}</Text>
      ) : history.isError ? (
        <Card>
          <Text role="alert">{t('training.historyError')}</Text>
          <Button onClick={() => void history.refetch()}>
            {t('trainer.retry')}
          </Button>
        </Card>
      ) : items.length === 0 ? (
        <Text>{t('training.historyEmpty')}</Text>
      ) : (
        <ol className={styles.history}>
          {items.map((attempt) => (
            <HistoryItem key={attempt.sequence} attempt={attempt} />
          ))}
        </ol>
      )}
      {history.hasNextPage && (
        <Button
          disabled={history.isFetchingNextPage}
          onClick={() => void history.fetchNextPage()}
        >
          {t(
            history.isFetchingNextPage
              ? 'training.historyLoading'
              : 'training.showMore',
          )}
        </Button>
      )}
    </section>
  )
}
function HistoryItem({ attempt }: { attempt: TrainingAttempt }) {
  const { t } = useTranslation()
  return (
    <li>
      <Text>
        {t('training.roundCount', {
          round: attempt.round,
          count: attempt.sequence,
        })}
        : {attempt.score} / {attempt.max_score}
      </Text>
      <Text as="p">{attempt.text}</Text>
      <Text as="p">{attempt.feedback.join(' ')}</Text>
    </li>
  )
}
