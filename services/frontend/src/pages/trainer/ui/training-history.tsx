import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TrainingAttempt, TrainingProgress } from '@/entities/training'
import { useTrainingHistoryQuery } from '@/features/training'
import styles from './trainer-layout.module.scss'
import { TrainingChat } from './training-chat'

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
            <HistoryItem
              key={attempt.sequence}
              attempt={attempt}
              userId={userId}
              sessionId={progress.session_id}
            />
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
function HistoryItem({
  attempt,
  userId,
  sessionId,
}: {
  attempt: TrainingAttempt
  userId: string
  sessionId: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const chatId = `history-chat-${attempt.exercise_id}`
  return (
    <li>
      <Text>
        {t('training.roundCount', {
          round: attempt.round,
          count: attempt.sequence,
        })}
      </Text>
      <Text as="p">{attempt.text}</Text>
      {attempt.answer_source ? (
        <>
          <Button
            variant="soft"
            aria-expanded={open}
            aria-controls={chatId}
            onClick={() => setOpen((value) => !value)}
          >
            {t(open ? 'training.chatHideHistory' : 'training.chatOpenHistory')}
          </Button>
          {open ? (
            <div id={chatId} className={styles.historyChat}>
              <TrainingChat
                userId={userId}
                sessionId={sessionId}
                exerciseId={attempt.exercise_id}
                readOnly
                sectionId={`${chatId}-title`}
              />
            </div>
          ) : null}
        </>
      ) : null}
    </li>
  )
}
