import { Button, Heading, Text } from '@radix-ui/themes'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError, type TrainingProgress } from '@/entities/training'
import { isMockApi } from '@/shared/api'
import { useTrainingSession, useTrainingVoice } from '@/features/training'
import { TrainingHistory } from './training-history'
import { TrainingChat } from './training-chat'
import {
  ListenButton,
  TrainerQuestion,
  TrainerShell,
  TrainerWorkspace,
  sessionStyles,
} from './trainer-shared'
import styles from './trainer-layout.module.scss'

function errorCode(error: unknown) {
  if (!(error instanceof TrainingApiError)) return 'training.skipError'
  if (error.status === 401) return 'trainer.unauthorized'
  if (error.status === 404) return 'training.sessionMissing'
  if (error.status === 409) return 'training.conflict'
  return 'training.skipError'
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
  const voice = useTrainingVoice(
    userId,
    progress,
    async (submission, signal) =>
      resource.submit
        .mutateAsync({ submission, signal })
        .then((result) => result.progress),
    onError,
  )
  const [missing, setMissing] = useState(false)
  const [skipFailed, setSkipFailed] = useState(false)
  const [chatRecording, setChatRecording] = useState(false)
  useEffect(() => {
    if (
      resource.query.error instanceof TrainingApiError &&
      resource.query.error.status === 404
    )
      setMissing(true)
  }, [resource.query.error])

  useEffect(() => {
    void voice.speak({ auto: true })
    // Instruction audio belongs to the exercise; tutor messages never submit an attempt.
  }, [progress.current.exercise_id])

  const unavailable =
    missing || resource.query.isPending || resource.query.isError
  const apiError = voice.error instanceof TrainingApiError ? voice.error : null
  const canSkip = !resource.submit.isPending

  async function skip() {
    setSkipFailed(false)
    try {
      await voice.skip()
    } catch {
      setSkipFailed(true)
    }
  }

  return (
    <TrainerShell
      title={t('training.title')}
      subject={progress.subject_name}
      crumb={t('training.roundCount', {
        round: progress.round,
        count: progress.answer_count + 1,
      })}
      onBack={unavailable ? undefined : onBack}
    >
      {unavailable ? (
        <main className={styles.notice}>
          {missing ? (
            <>
              <Text role="alert">{t('training.sessionMissing')}</Text>
              <Button
                onClick={() => {
                  onMissing()
                  onBack()
                }}
              >
                {t('home.back')}
              </Button>
            </>
          ) : resource.query.isPending ? (
            <Text role="status">{t('training.sessionLoading')}</Text>
          ) : (
            <>
              <Text role="alert">{t('training.sessionError')}</Text>
              <Button onClick={() => void resource.query.refetch()}>
                {t('trainer.retry')}
              </Button>
            </>
          )}
        </main>
      ) : (
        <TrainerWorkspace variant="training">
          <TrainerQuestion
            badge={t('session.practice')}
            eyebrow={progress.current.outcome_name}
            eyebrowHeading
            question={progress.current.question}
            options={progress.current.options}
          >
            {isMockApi() ? (
              <Text as="p" role="note" className={sessionStyles.note}>
                {t('training.demoNotice')}
              </Text>
            ) : null}
            <Text as="p" className={sessionStyles.note}>
              {progress.current.voice_instruction}
            </Text>
            <ListenButton
              disabled={chatRecording}
              state={
                voice.speaking
                  ? 'playing'
                  : voice.audioStatus === 'loading'
                    ? 'loading'
                    : 'idle'
              }
              onClick={() => void voice.speak()}
            />
            {voice.audioStatus !== 'idle' &&
            voice.audioStatus !== 'ready' &&
            voice.audioStatus !== 'loading' ? (
              <Text role="status" className={sessionStyles.hint}>
                {t(`training.audio.${voice.audioStatus}`)}
              </Text>
            ) : null}
            {voice.speechError ? (
              <Text role="alert" className={sessionStyles.hint}>
                {t('trainer.speechError')}
              </Text>
            ) : null}
            <TrainingChat
              userId={userId}
              sessionId={progress.session_id}
              exerciseId={progress.current.exercise_id}
              onRecordingStart={voice.stopSpeech}
              onRecordingChange={setChatRecording}
            />
            <section className={sessionStyles.trainingDetails}>
              <Heading as="h2">{t('training.targets')}</Heading>
              <ul>
                {progress.targets.map((target, index) => (
                  <li key={`${target.outcome_id}-${index}`}>
                    <Text>{target.outcome_name}</Text>
                  </li>
                ))}
              </ul>
              {skipFailed ? (
                <Text role="alert">
                  {t(apiError ? errorCode(apiError) : 'session.skipError')}
                </Text>
              ) : null}
              <TrainingHistory userId={userId} progress={progress} />
              <Button
                className={sessionStyles.primary}
                disabled={!canSkip}
                onClick={() => void skip()}
              >
                {t('training.skipExercise')}
              </Button>
            </section>
          </TrainerQuestion>
        </TrainerWorkspace>
      )}
    </TrainerShell>
  )
}
