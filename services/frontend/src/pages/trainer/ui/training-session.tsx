import { Button, Flex, Heading, Text } from '@radix-ui/themes'
import { Mic, RotateCcw } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError, type TrainingProgress } from '@/entities/training'
import { isMockApi } from '@/shared/api'
import { useTrainingSession, useTrainingVoice } from '@/features/training'
import { TrainingHistory } from './training-history'
import {
  ListenButton,
  QuestionNavigator,
  TrainerCapturePanel,
  TrainerQuestion,
  TrainerShell,
  TrainerTranscript,
  TrainerWorkspace,
  sessionStyles,
} from './trainer-shared'
import type { NavigatorItem } from './trainer-shared'
import {
  initialTrack,
  readTrack,
  recordTrainingAnswer,
  writeTrack,
} from '../model/question-track'
import type { QuestionTrack } from '../model/question-track'
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

// Training is endless: answered exercises are shown as done without their
// score and the open exercise is current.
function useTrainingTrack(
  progress: TrainingProgress,
  result: TrainingProgress['answer'] | undefined,
): NavigatorItem[] {
  const id = progress.session_id
  const store = useRef<{ id?: string; track: QuestionTrack }>({
    track: initialTrack(0, 0),
  })
  if (store.current.id !== id)
    store.current = {
      id,
      track: readTrack(id) ?? initialTrack(progress.answer_count, 0),
    }
  const [, rerender] = useState(0)
  useEffect(() => {
    if (!result) return
    const next = recordTrainingAnswer(
      store.current.track,
      result.sequence,
      result.score,
      result.max_score,
      result.skipped,
    )
    store.current = { id, track: next }
    writeTrack(id, next)
    rerender((value) => value + 1)
  }, [id, result])
  const answered = Math.max(
    progress.answer_count,
    store.current.track.states.length,
  )
  const items: NavigatorItem[] = Array.from({ length: answered }, (_, i) => {
    const state = store.current.track.states[i]
    return {
      number: i + 1,
      state: state === 'skipped' ? state : 'done',
    }
  })
  if (!result) items.push({ number: answered + 1, state: 'current' })
  return items
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
  const track = useTrainingTrack(progress, result)
  const [skipFailed, setSkipFailed] = useState(false)
  async function skip() {
    setSkipFailed(false)
    try {
      await voice.skip()
    } catch {
      setSkipFailed(true)
    }
  }
  const unavailable =
    missing || resource.query.isPending || resource.query.isError
  const micError =
    voice.captureError === 'denied' || voice.captureError === 'unavailable'
      ? {
          title: t('trainer.micFailedTitle'),
          help: t('trainer.micFailedHelp'),
        }
      : undefined
  return (
    <TrainerShell
      title={t('training.title')}
      subject={progress.subject_name}
      crumb={t('training.roundCount', {
        round: result?.round ?? progress.round,
        count: result?.sequence ?? progress.answer_count + 1,
      })}
      onBack={unavailable ? undefined : goBack}
      navigator={
        unavailable ? undefined : (
          <QuestionNavigator
            label={t('training.answersNavigator')}
            items={track}
          />
        )
      }
    >
      {unavailable ? (
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
          <TrainerWorkspace>
            <TrainerQuestion
              badge={t('session.practice')}
              eyebrow={exercise.outcome_name}
              eyebrowHeading
              question={exercise.question}
              options={exercise.options}
              instruction={
                <>
                  <ListenButton
                    state={
                      voice.speaking
                        ? 'playing'
                        : !result && voice.audioStatus === 'loading'
                          ? 'loading'
                          : 'idle'
                    }
                    disabled={!result && (recording || processing || waiting)}
                    onClick={() => void voice.speak()}
                  />
                  {!result &&
                    voice.audioStatus !== 'idle' &&
                    voice.audioStatus !== 'ready' &&
                    voice.audioStatus !== 'loading' && (
                      <Text role="status" className={sessionStyles.hint}>
                        {t(`training.audio.${voice.audioStatus}`)}
                      </Text>
                    )}
                  {voice.speechError && (
                    <Text role="alert" className={sessionStyles.hint}>
                      {t('trainer.speechError')}
                    </Text>
                  )}
                </>
              }
            >
              {isMockApi() && (
                <Text as="p" role="note" className={sessionStyles.note}>
                  {t('training.demoNotice')}
                </Text>
              )}
              <Text as="p" className={sessionStyles.note}>
                {exercise.voice_instruction}
              </Text>
              <TrainerTranscript
                id="training-transcript-title"
                text={result?.text}
              />
              <div className={sessionStyles.extra}>
                <section>
                  <Heading as="h2">{t('training.targets')}</Heading>
                  <ul>
                    {progress.targets.map((target, index) => (
                      <li key={`${target.outcome_id}-${index}`}>
                        <Text>{target.outcome_name}</Text>
                      </li>
                    ))}
                  </ul>
                </section>
                <TrainingHistory userId={userId} progress={progress} />
              </div>
            </TrainerQuestion>
            {result ? (
              <TrainerCapturePanel
                stage="ready"
                seconds={seconds}
                answered={result.skipped ? 'skipped' : 'accepted'}
              >
                <Button className={sessionStyles.primary} onClick={voice.next}>
                  {t('session.next')}
                </Button>
              </TrainerCapturePanel>
            ) : (
              <>
                <TrainerCapturePanel
                  stage={voice.stage === 'result' ? 'ready' : voice.stage}
                  seconds={seconds}
                  stream={voice.stream}
                  errorMessage={
                    skipFailed
                      ? t('session.skipError')
                      : error
                        ? t(
                            voice.error
                              ? errorCode(voice.error)
                              : captureErrorKey,
                          )
                        : undefined
                  }
                  micError={micError}
                  onMic={
                    // The circle only starts or stops a recording; recovery
                    // actions keep their explicit buttons.
                    processing ||
                    waiting ||
                    (error &&
                      (refreshCurrent ||
                        resetReservation ||
                        (voice.hasPending && !rerecord)))
                      ? undefined
                      : recording
                        ? () => void voice.stop()
                        : error
                          ? recordAgain
                          : () => void voice.start()
                  }
                  onSkip={() => void skip()}
                  skipDisabled={recording || processing || waiting}
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
                    <Button
                      className={sessionStyles.primary}
                      onClick={recordAgain}
                    >
                      <Mic size={16} aria-hidden="true" />
                      {t(micError ? 'trainer.retry' : 'diagnostic.recordAgain')}
                    </Button>
                  ) : // Recording starts and stops from the microphone circle.
                  null}
                </TrainerCapturePanel>
              </>
            )}
          </TrainerWorkspace>
        </>
      )}
    </TrainerShell>
  )
}
