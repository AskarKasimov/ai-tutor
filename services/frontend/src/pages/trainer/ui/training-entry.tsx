import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError, type TrainingProgress } from '@/entities/training'
import {
  useTrainingEntryQuery,
  useStartTrainingMutation,
} from '@/features/training'
import { TrainingSession } from './training-session'
import { TrainerShell } from './trainer-shared'
import styles from './trainer-layout.module.scss'

export function TrainingEntry({
  userId,
  diagnosticId,
  subjectName,
  onBack,
}: {
  userId: string
  diagnosticId: string
  subjectName: string
  onBack: () => void
}) {
  const { t } = useTranslation()
  const entry = useTrainingEntryQuery(userId, diagnosticId)
  const start = useStartTrainingMutation(userId, diagnosticId)
  const [started, setStarted] = useState<TrainingProgress>()
  const autoStarted = useRef(false)
  const preview =
    entry.data?.kind === 'preview' ? entry.data.preview : undefined
  const { mutate } = start
  useEffect(() => {
    if (
      !preview ||
      preview.status === 'no_practice_tasks' ||
      autoStarted.current
    )
      return
    autoStarted.current = true
    mutate({ preview }, { onSuccess: (result) => setStarted(result.progress) })
  }, [preview, mutate])

  const progress =
    started ??
    (entry.data?.kind === 'existing' ? entry.data.progress : undefined)
  if (progress)
    return (
      <TrainingSession
        key={progress.session_id}
        userId={userId}
        initial={progress}
        onBack={onBack}
        onMissing={() => setStarted(undefined)}
      />
    )

  const missing =
    entry.error instanceof TrainingApiError && entry.error.status === 404
  const stale =
    start.error instanceof TrainingApiError && start.error.status === 409
  const error = entry.error ?? start.error
  const noTasks = preview?.status === 'no_practice_tasks'
  return (
    <TrainerShell
      title={t('training.title')}
      subject={subjectName}
      onBack={onBack}
    >
      <main className={styles.preview}>
        <Heading as="h1">{t('training.title')}</Heading>
        {missing ? (
          <>
            <Text role="alert">{t('training.sessionMissing')}</Text>
            <Button onClick={onBack}>{t('home.back')}</Button>
          </>
        ) : noTasks ? (
          <Text role="status">{t('training.noPracticeTasks')}</Text>
        ) : error ? (
          <Card>
            <Text role="alert">
              {t(
                error instanceof TrainingApiError && error.status === 401
                  ? 'trainer.unauthorized'
                  : stale
                    ? 'training.stalePreview'
                    : entry.isError
                      ? 'training.previewError'
                      : 'training.startError',
              )}
            </Text>
            <Button
              disabled={entry.isFetching || start.isPending}
              onClick={() => {
                if (entry.isError) void entry.refetch()
                else if (preview)
                  mutate(
                    { preview },
                    { onSuccess: (result) => setStarted(result.progress) },
                  )
              }}
            >
              {t('trainer.retry')}
            </Button>
          </Card>
        ) : (
          <Text role="status">{t('training.starting')}</Text>
        )}
      </main>
    </TrainerShell>
  )
}
