import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { Mic, Square, Volume2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError, type TrainingProgress } from '@/entities/training'
import { isMockApi } from '@/shared/api'
import { useTrainingSession, useTrainingVoice } from '@/features/training'
import { AccountMenu } from '@/features/auth'
import { VoiceIllustration } from './trainer-answer'
import { TrainingHistory } from './training-history'
import styles from './training.module.scss'

function errorCode(error: unknown) {
  if (!(error instanceof TrainingApiError)) return 'training.submitError'
  if (error.status === 401) return 'trainer.unauthorized'
  if (error.status === 404) return 'training.sessionMissing'
  if (error.status === 409) {
    if (error.code === 'TRAINING_ANSWER_IN_PROGRESS')
      return 'training.answerInProgress'
    if (error.code === 'TRAINING_ANSWER_RETRY_MISMATCH')
      return 'training.retryMismatch'
    if (error.code === 'TRAINING_EXERCISE_NOT_CURRENT')
      return 'training.exerciseChanged'
    return 'training.conflict'
  }
  return 'training.submitError'
}

export function TrainingSession({
  userId,
  initial,
  onBack,
  onMissing,
}: {
  userId: string
  initial: TrainingProgress
  onBack: () => void
  onMissing: () => void
}) {
  const { t } = useTranslation()
  const resource = useTrainingSession(userId, initial.session_id)
  const progress = resource.query.data ?? initial
  const onError = useCallback(() => {}, [])
  const [missing, setMissing] = useState(false)
  useEffect(() => {
    if (
      resource.query.error instanceof TrainingApiError &&
      resource.query.error.status === 404
    )
      setMissing(true)
  }, [resource.query.error])
  const voice = useTrainingVoice(
    userId,
    progress,
    async (submission, signal) =>
      resource.submit
        .mutateAsync({ submission, signal })
        .then((result) => result.progress),
    onError,
  )
  if (missing)
    return (
      <main className={styles.page}>
        <Text role="alert">{t('training.sessionMissing')}</Text>
        <Button
          onClick={() => {
            onMissing()
            onBack()
          }}
        >
          {t('home.back')}
        </Button>
      </main>
    )
  if (resource.query.isPending)
    return (
      <main className={styles.page}>
        <Text role="status">{t('training.sessionLoading')}</Text>
      </main>
    )
  if (resource.query.isError)
    return (
      <main className={styles.page}>
        <Text role="alert">{t('training.sessionError')}</Text>
        <Button onClick={() => void resource.query.refetch()}>
          {t('trainer.retry')}
        </Button>
      </main>
    )
  const accepted = voice.accepted
  const exercise = accepted?.exercise ?? progress.current
  const result = accepted?.progress.answer
  const error = voice.error ?? voice.captureError
  const captureErrorKey =
    voice.captureError === 'denied'
      ? 'trainer.denied'
      : voice.captureError === 'unavailable'
        ? 'trainer.unavailable'
        : voice.captureError === 'empty'
          ? 'trainer.transcription'
          : 'trainer.capture'
  const refreshCurrent =
    voice.error instanceof TrainingApiError &&
    voice.error.status === 409 &&
    voice.error.code === 'TRAINING_EXERCISE_NOT_CURRENT'
  const statusKey =
    voice.stage === 'permission'
      ? 'trainer.allow'
      : voice.stage === 'recording'
        ? 'trainer.recording'
        : voice.stage === 'processing'
          ? 'trainer.processing'
          : voice.stage === 'error'
            ? 'trainer.errorStatus'
            : 'training.ready'
  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <div>
          <Text>{progress.subject_name}</Text>
          <Heading as="h1">{t('training.title')}</Heading>
        </div>
        <AccountMenu />
      </header>
      <Button
        variant="soft"
        onClick={() => {
          if (voice.stage === 'recording') voice.next()
          onBack()
        }}
      >
        {t('home.back')}
      </Button>
      {isMockApi() && <Text role="note">{t('training.demoNotice')}</Text>}
      <section className={styles.columns}>
        <Card className={styles.question}>
          <Text>
            {t('training.roundCount', {
              round: result?.round ?? progress.round,
              count: result?.sequence ?? progress.answer_count + 1,
            })}
          </Text>
          <Heading as="h2">{exercise.outcome_name}</Heading>
          <Text as="p">{exercise.question}</Text>
          <ol>
            {exercise.options.map((option, i) => (
              <li key={`${i}-${option}`}>{option}</li>
            ))}
          </ol>
          <Text as="p">{exercise.voice_instruction}</Text>
          <Button
            onClick={() => void voice.speak()}
            disabled={!!accepted || voice.stage === 'permission'}
          >
            <Volume2 size={16} /> {t('trainer.playInstruction')}
          </Button>
          {voice.audioStatus !== 'idle' && voice.audioStatus !== 'ready' && (
            <Text role="status">
              {t(`training.audio.${voice.audioStatus}`)}
            </Text>
          )}
          {voice.speechError && (
            <Text role="status">{t('trainer.speechError')}</Text>
          )}
          {voice.stage !== 'ready' && voice.stage !== 'result' && (
            <Text role="status">
              {t(
                statusKey,
                voice.stage === 'recording'
                  ? { time: voice.seconds }
                  : undefined,
              )}
            </Text>
          )}
          <div className={styles.voice}>
            <VoiceIllustration stream={voice.stream} />
            {voice.stage === 'recording' ? (
              <Button onClick={() => void voice.stop()}>
                <Square size={16} /> {t('trainer.stopRecording')} ·{' '}
                {voice.seconds}s
              </Button>
            ) : voice.stage === 'processing' ? (
              <Text role="status">{t('trainer.processing')}</Text>
            ) : voice.stage === 'result' ? (
              <Button onClick={voice.next}>{t('training.next')}</Button>
            ) : (
              <Button
                onClick={() => void voice.start()}
                disabled={voice.stage === 'permission'}
              >
                <Mic size={16} />{' '}
                {t(
                  voice.stage === 'permission'
                    ? 'trainer.allow'
                    : 'trainer.start',
                )}
              </Button>
            )}
          </div>
          {error && (
            <Card>
              <Text role="alert">
                {t(voice.error ? errorCode(voice.error) : captureErrorKey)}
              </Text>
              {refreshCurrent ? (
                <Button onClick={() => void resource.query.refetch()}>
                  {t('training.refresh')}
                </Button>
              ) : voice.hasPending ? (
                <Button onClick={() => void voice.retry()}>
                  {t('training.retrySubmit')}
                </Button>
              ) : (
                <Button onClick={() => void voice.start()}>
                  {t('trainer.retry')}
                </Button>
              )}
            </Card>
          )}
          {voice.answerUrl && (
            <audio
              controls
              src={voice.answerUrl}
              aria-label={t('trainer.listenRecording')}
            />
          )}
          {result && (
            <section aria-live="polite">
              <Heading as="h3">
                {t('training.score', {
                  score: result.score,
                  max: result.max_score,
                })}
              </Heading>
              <Text as="p">{accepted?.exercise.question}</Text>
              <Text as="p">{result.text}</Text>
              {result.feedback.map((line, i) => (
                <Text as="p" key={`${i}-${line}`}>
                  {line}
                </Text>
              ))}
            </section>
          )}
        </Card>
        <Card>
          <Heading as="h2">{t('training.targets')}</Heading>
          <ul>
            {progress.targets.map((target, index) => (
              <li key={`${target.outcome_id}-${index}`}>
                <Text>{target.outcome_name}</Text>
                {target.last_score !== null && (
                  <Text> · {target.last_score}</Text>
                )}
              </li>
            ))}
          </ul>
          <Text>
            {t('training.latestScore', {
              score: progress.answer?.score ?? '—',
            })}
          </Text>
        </Card>
      </section>
      <TrainingHistory userId={userId} progress={progress} />
    </main>
  )
}
