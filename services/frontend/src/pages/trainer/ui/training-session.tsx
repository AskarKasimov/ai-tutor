import { Button, Flex, Heading, Text } from '@radix-ui/themes'
import { Mic, RotateCcw, Square } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError, type TrainingProgress } from '@/entities/training'
import { isMockApi } from '@/shared/api'
import { useTrainingSession, useTrainingVoice } from '@/features/training'
import { TrainingHistory } from './training-history'
import {
  TrainerAnswerPanel,
  TrainerCapturePanel,
  TrainerQuestion,
  TrainerScore,
  TrainerShell,
} from './trainer-shared'
import styles from './trainer-layout.module.scss'

function errorCode(error: unknown) {
  if (!(error instanceof TrainingApiError)) return 'training.submitError'
  if (error.status === 401) return 'trainer.unauthorized'
  if (error.status === 404) return 'training.sessionMissing'
  if (error.status === 413) return 'diagnostic.recordingLimit'
  if (error.code === 'NO_SPEECH_DETECTED') return 'diagnostic.noSpeech'
  if (error.status === 415 || error.status === 422)
    return 'diagnostic.invalidAudio'
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
  const accepted = voice.accepted
  const exercise = accepted?.exercise ?? progress.current
  const openExerciseId = accepted ? undefined : progress.current.exercise_id
  // Read the stored voice instruction aloud whenever a new exercise opens.
  useEffect(() => {
    if (openExerciseId) void voice.speak({ auto: true })
    // voice is recreated on every render; autoplay runs once per exercise.
  }, [openExerciseId])
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
  const resetReservation =
    voice.error instanceof TrainingApiError &&
    voice.error.code === 'TRAINING_ANSWER_RETRY_MISMATCH'
  const apiError = voice.error instanceof TrainingApiError ? voice.error : null
  // Rejected audio needs a fresh recording; technical failures also allow one.
  const rerecord = !!apiError && [413, 415, 422].includes(apiError.status)
  const canAlsoRerecord =
    voice.hasPending && !rerecord && (!apiError || apiError.status >= 500)
  const recordAgain = () => {
    voice.next()
    void voice.start()
  }
  const recording = voice.stage === 'recording'
  const processing = voice.stage === 'processing'
  const waiting = voice.stage === 'permission'
  const seconds = `${Math.floor(voice.seconds / 60)
    .toString()
    .padStart(2, '0')}:${(voice.seconds % 60).toString().padStart(2, '0')}`
  const goBack = () => {
    if (recording) voice.next()
    onBack()
  }
  return (
    <TrainerShell
      title={t('training.title')}
      subject={progress.subject_name}
      progress={{
        label: t('training.roundCount', {
          round: result?.round ?? progress.round,
          count: result?.sequence ?? progress.answer_count + 1,
        }),
      }}
    >
      {missing || resource.query.isPending || resource.query.isError ? (
        <main className={styles.notice}>
          {missing ? (
            <>
              <Text role="alert">{t('training.sessionMissing')}</Text>
              <Button
                onClick={() => {
                  onMissing()
                  goBack()
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
        <>
          <Button variant="soft" onClick={goBack}>
            {t('home.back')}
          </Button>
          {isMockApi() && <Text role="note">{t('training.demoNotice')}</Text>}
          <main className={styles.workspace}>
            <TrainerQuestion
              badge={t('session.practice')}
              eyebrow={exercise.outcome_name}
              eyebrowHeading
              question={exercise.question}
              options={exercise.options}
            >
              <Text as="p" className={styles.trainingInstruction}>
                {exercise.voice_instruction}
              </Text>
              <section
                className={styles.savedAnswer}
                aria-labelledby="training-transcript-title"
              >
                <Heading
                  as="h2"
                  id="training-transcript-title"
                  className={styles.eyebrow}
                >
                  {t('diagnostic.transcript')}
                </Heading>
                <Text as="p" className={styles.transcript}>
                  {result?.text ?? t('diagnostic.transcriptPending')}
                </Text>
              </section>
            </TrainerQuestion>
            {result ? (
              <TrainerAnswerPanel>
                <Text as="p" className={styles.eyebrow}>
                  {t('session.result')}
                </Text>
                <TrainerScore
                  score={result.score}
                  maxScore={result.max_score}
                  verdict={t(`diagnostic.verdict.${result.verdict}`)}
                />
                <Heading as="h2" className={styles.feedbackTitle}>
                  {t('training.score', {
                    score: result.score,
                    max: result.max_score,
                  })}
                </Heading>
                <Text as="p">{accepted?.exercise.question}</Text>
                {result.feedback.map((line, i) => (
                  <Text
                    as="p"
                    key={`${i}-${line}`}
                    className={styles.feedbackLine}
                  >
                    {line}
                  </Text>
                ))}
                {voice.answerUrl && (
                  <div className={styles.audio}>
                    <audio
                      controls
                      src={voice.answerUrl}
                      aria-label={t('trainer.listenRecording')}
                    />
                  </div>
                )}
                <div className={styles.resultActions}>
                  <Button className={styles.primary} onClick={voice.next}>
                    {t('training.next')}
                  </Button>
                </div>
              </TrainerAnswerPanel>
            ) : (
              <>
                <div className={styles.repeatControl}>
                  <Button
                    className={styles.repeat}
                    variant="soft"
                    disabled={recording || processing || waiting}
                    onClick={() => void voice.speak()}
                  >
                    <RotateCcw size={18} aria-hidden="true" />{' '}
                    {t('trainer.playInstruction')}
                  </Button>
                  {voice.audioStatus !== 'idle' &&
                    voice.audioStatus !== 'ready' && (
                      <Text role="status" className={styles.speechError}>
                        {t(`training.audio.${voice.audioStatus}`)}
                      </Text>
                    )}
                  {voice.speechError && (
                    <Text role="alert" className={styles.speechError}>
                      {t('trainer.speechError')}
                    </Text>
                  )}
                </div>
                <TrainerCapturePanel
                  stage={voice.stage === 'result' ? 'ready' : voice.stage}
                  seconds={seconds}
                  stream={voice.stream}
                  readyHelp="diagnostic.answerHelp"
                  errorMessage={
                    error
                      ? t(
                          voice.error
                            ? errorCode(voice.error)
                            : captureErrorKey,
                        )
                      : undefined
                  }
                  audioUrl={voice.answerUrl}
                >
                  {error && refreshCurrent ? (
                    <Button
                      className={styles.primary}
                      onClick={() => void resource.query.refetch()}
                    >
                      {t('training.refresh')}
                    </Button>
                  ) : error && resetReservation ? (
                    <Button
                      className={styles.primary}
                      onClick={() => void voice.reset()}
                    >
                      {t('training.resetSubmit')}
                    </Button>
                  ) : error && voice.hasPending && !rerecord ? (
                    <Flex gap="4" align="center" wrap="wrap">
                      <Button
                        className={styles.primary}
                        onClick={() => void voice.retry()}
                      >
                        <RotateCcw size={16} aria-hidden="true" />
                        {t('training.retrySubmit')}
                      </Button>
                      {canAlsoRerecord && (
                        <Button
                          variant="ghost"
                          color="gray"
                          onClick={recordAgain}
                        >
                          {t('diagnostic.recordAgain')}
                        </Button>
                      )}
                    </Flex>
                  ) : error ? (
                    <Button className={styles.primary} onClick={recordAgain}>
                      <Mic size={16} aria-hidden="true" />
                      {t('diagnostic.recordAgain')}
                    </Button>
                  ) : (
                    <Button
                      className={styles.primary}
                      disabled={processing || waiting}
                      onClick={() =>
                        void (recording ? voice.stop() : voice.start())
                      }
                    >
                      {recording && <Square size={16} aria-hidden="true" />}
                      {t(
                        recording
                          ? 'trainer.stopRecording'
                          : waiting
                            ? 'trainer.allow'
                            : 'trainer.start',
                      )}
                      {recording && ` · ${voice.seconds}s`}
                    </Button>
                  )}
                </TrainerCapturePanel>
              </>
            )}
          </main>
          <div className={styles.trainingDetails}>
            <section>
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
            </section>
            <TrainingHistory userId={userId} progress={progress} />
          </div>
        </>
      )}
    </TrainerShell>
  )
}
