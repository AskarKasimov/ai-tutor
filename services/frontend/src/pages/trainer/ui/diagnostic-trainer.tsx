import { Button, Heading, Text } from '@radix-ui/themes'
import {
  GraduationCap,
  LoaderCircle,
  Mic,
  RotateCcw,
  Slash,
  Square,
} from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import {
  useDiagnosticResultQuery,
  useDiagnosticSession,
} from '@/features/diagnostic-session'
import { useDiagnosticVoice } from '@/features/diagnostic-session'
import type {
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'
import { AccountMenu } from '@/features/auth'
import { VoiceIllustration } from '@/pages/trainer/ui/trainer-answer'
import styles from '@/pages/trainer/ui/trainer-layout.module.scss'
import diagnosticStyles from '@/pages/trainer/ui/diagnostic.module.scss'

function errorKey(error: unknown, fallback: string) {
  if (!(error instanceof DiagnosticApiError)) return fallback
  if (error.code === 'DIAGNOSTIC_ANSWER_IN_PROGRESS')
    return 'diagnostic.answerInProgress'
  if (error.code === 'NO_ELIGIBLE_COMPETENCIES') return 'diagnostic.unavailable'
  if (error.status === 401) return 'trainer.unauthorized'
  if (error.status === 404) return 'diagnostic.expired'
  if (error.status === 409) return 'diagnostic.conflict'
  if (error.status === 413) return 'diagnostic.recordingLimit'
  if (error.status === 415 || error.status === 422)
    return 'diagnostic.invalidAudio'
  if (error.code === 'INVALID_RESPONSE') return 'diagnostic.invalidResponse'
  return fallback
}
export function DiagnosticTrainer() {
  const { user } = useAuth()
  return user ? <StudentDiagnostic key={user.id} userId={user.id} /> : null
}
function StudentDiagnostic({ userId }: { userId: string }) {
  const { t } = useTranslation()
  const training = useDiagnosticSession(userId)
  const progress = training.query.data
  return (
    <div data-trainer-app className={styles.trainerLayout}>
      <aside className={styles.navigation}>
        <div className={styles.brand}>
          <span className={styles.brandIcon} aria-hidden="true">
            <GraduationCap size={24} />
          </span>
          <Text>{t('trainer.title')}</Text>
        </div>
        <div className={styles.courseHeading}>
          <Text as="p" className={styles.eyebrow}>
            {t('diagnostic.title')}
          </Text>
          <Heading as="h2">{t('session.course')}</Heading>
        </div>
        {progress && (
          <div className={styles.progress}>
            <Text as="p" aria-live="polite">
              {t('diagnostic.progress', {
                completed: progress.completed_tasks,
                skipped: progress.skipped_tasks,
                total: progress.total_tasks,
              })}
            </Text>
            <div
              className={styles.progressTrack}
              role="progressbar"
              aria-label={t('session.progressLabel')}
              aria-valuemin={0}
              aria-valuemax={progress.total_tasks}
              aria-valuenow={progress.completed_tasks + progress.skipped_tasks}
            >
              <span
                className={styles.progressFill}
                style={{
                  width: `${((progress.completed_tasks + progress.skipped_tasks) / progress.total_tasks) * 100}%`,
                }}
              />
            </div>
          </div>
        )}
      </aside>
      <div className={styles.content}>
        <header className={styles.header}>
          <div className={styles.breadcrumb}>
            <Text>{t('diagnostic.title')}</Text>
            <span aria-hidden="true">
              <Slash size={14} />
            </span>
            <Text weight="bold">{t('session.course')}</Text>
          </div>
          <AccountMenu />
        </header>
        {training.query.isPending ? (
          <main className={styles.notice}>
            <Text role="status">{t('session.loading')}</Text>
          </main>
        ) : training.query.isError ? (
          <main className={styles.notice}>
            <Text role="alert">
              {t(
                errorKey(
                  training.query.error,
                  training.query.error instanceof DiagnosticApiError &&
                    training.query.error.status === 422
                    ? 'diagnostic.unavailable'
                    : 'session.loadError',
                ),
              )}
            </Text>
            {training.query.error instanceof DiagnosticApiError &&
            training.query.error.status === 404 ? (
              <Button
                disabled={training.restart.isPending}
                onClick={() => training.restart.mutate()}
              >
                {t('session.restart')}
              </Button>
            ) : (
              <Button
                disabled={training.query.isFetching}
                onClick={() => void training.query.refetch()}
              >
                {t('trainer.retry')}
              </Button>
            )}
            {training.restart.isError && (
              <Text role="alert">{t('session.loadError')}</Text>
            )}
          </main>
        ) : progress ? (
          <DiagnosticFlow
            key={progress.session_id}
            userId={userId}
            training={training}
            progress={progress}
          />
        ) : null}
      </div>
    </div>
  )
}
function DiagnosticFlow({
  userId,
  training,
  progress,
}: {
  userId: string
  training: ReturnType<typeof useDiagnosticSession>
  progress: DiagnosticProgress
}) {
  const { t } = useTranslation()
  const voice = useDiagnosticVoice(
    userId,
    progress.session_id,
    progress.current,
    (submission, signal) => training.submit.mutateAsync({ submission, signal }),
    training.handleError,
  )
  const task = voice.accepted?.task ?? progress.current
  if (progress.status === 'completed' && !voice.accepted)
    return (
      <DiagnosticSummary
        userId={userId}
        sessionId={progress.session_id}
        training={training}
      />
    )
  if (!task)
    return (
      <main className={styles.notice}>
        <Text>{t('session.empty')}</Text>
      </main>
    )
  const recording = voice.stage === 'recording'
  const processing = voice.stage === 'processing'
  const waiting = voice.stage === 'permission'
  const blocked = recording || processing || waiting
  const failed = voice.stage === 'error'
  const seconds = `${Math.floor(voice.seconds / 60)
    .toString()
    .padStart(2, '0')}:${(voice.seconds % 60).toString().padStart(2, '0')}`
  const response = voice.accepted?.progress
  async function refresh() {
    const refreshed = await training.query.refetch()
    if (
      !refreshed.error &&
      (refreshed.data?.status === 'completed' ||
        refreshed.data?.current?.variant_task_id !== voice.pendingTaskId)
    )
      voice.reset()
  }
  return (
    <main className={styles.workspace}>
      <Question task={task} transcript={response?.text} />
      {response ? (
        <aside
          className={styles.answerPanel}
          aria-label={t('trainer.answerArea')}
        >
          <Feedback
            score={response.score!}
            maxScore={response.grader_max_score!}
            verdict={response.verdict!}
            feedback={response.feedback!}
            criteria={response.criterion_results}
          />
          {voice.audioUrl && (
            <div className={styles.audio}>
              <Text as="p">{t('trainer.listenRecording')}</Text>
              <audio
                controls
                src={voice.audioUrl}
                aria-label={t('trainer.listenRecording')}
              />
            </div>
          )}
          <div className={styles.resultActions}>
            <Button className={styles.primary} onClick={voice.reset}>
              {t(
                progress.status === 'completed'
                  ? 'session.viewSummary'
                  : 'session.next',
              )}
            </Button>
          </div>
        </aside>
      ) : (
        <>
          <div className={styles.repeatControl}>
            <Button
              className={styles.repeat}
              variant="soft"
              disabled={blocked}
              onClick={() => void voice.speak()}
            >
              {voice.speech === 'loading' ? (
                <LoaderCircle
                  size={18}
                  className={styles.spinner}
                  aria-hidden="true"
                />
              ) : voice.speech === 'playing' ? (
                <Square size={16} aria-hidden="true" />
              ) : (
                <RotateCcw size={18} aria-hidden="true" />
              )}
              {t(
                voice.speech === 'loading'
                  ? 'trainer.cancelSpeech'
                  : voice.speech === 'playing'
                    ? 'trainer.stopSpeech'
                    : 'trainer.playInstruction',
              )}
            </Button>
            {voice.speechError && (
              <Text as="p" role="alert" className={styles.speechError}>
                {t('trainer.speechError')}
              </Text>
            )}
            {voice.pendingHint && (
              <Text as="p" role="status" className={styles.speechError}>
                {t('trainer.audioPreparing')}
              </Text>
            )}
            {voice.cancelledHint && (
              <Text as="p" role="status" className={styles.speechError}>
                {t('trainer.audioCancelled')}
              </Text>
            )}
          </div>
          <aside
            className={`${styles.answerPanel} ${failed ? styles.answerError : ''}`}
            aria-label={t('trainer.answerArea')}
          >
            <Text as="p" className={styles.eyebrow}>
              {t('trainer.yourAnswer')}
            </Text>
            <VoiceIllustration stream={voice.stream} />
            <Heading as="h2" className={styles.answerTitle}>
              {t(
                recording
                  ? 'trainer.recording'
                  : processing
                    ? 'trainer.processing'
                    : waiting
                      ? 'trainer.permission'
                      : 'trainer.answer',
                { time: seconds },
              )}
            </Heading>
            <Text as="p" className={styles.instructions}>
              {t(
                recording
                  ? 'trainer.recordingHelp'
                  : processing
                    ? 'trainer.processingHelp'
                    : waiting
                      ? 'trainer.permissionHelp'
                      : 'diagnostic.answerHelp',
              )}
            </Text>
            {failed && (
              <Text as="p" role="alert" className={styles.speechError}>
                {t(
                  voice.captureError
                    ? `trainer.${voice.captureError}`
                    : errorKey(voice.error, 'diagnostic.submitError'),
                )}
              </Text>
            )}
            {recording ? (
              <div className={styles.recordingTimer}>
                <span />
                {seconds}
              </div>
            ) : blocked ? (
              <div className={styles.processingIndicator}>
                <LoaderCircle
                  size={32}
                  className={styles.spinner}
                  aria-hidden="true"
                />
              </div>
            ) : (
              <div className={styles.microphone} aria-hidden="true">
                <Mic size={40} />
              </div>
            )}
            {voice.audioUrl && failed && (
              <div className={styles.audio}>
                <audio
                  controls
                  src={voice.audioUrl}
                  aria-label={t('trainer.listenRecording')}
                />
              </div>
            )}
            {failed &&
            voice.error instanceof DiagnosticApiError &&
            ((voice.error.status === 409 &&
              voice.error.code !== 'DIAGNOSTIC_ANSWER_IN_PROGRESS') ||
              voice.error.status === 404) ? (
              <Button className={styles.primary} onClick={() => void refresh()}>
                {t('diagnostic.refresh')}
              </Button>
            ) : (
              <Button
                className={styles.primary}
                disabled={processing || waiting}
                onClick={() =>
                  void (recording
                    ? voice.stop()
                    : voice.hasPending
                      ? voice.retry()
                      : voice.start())
                }
              >
                {t(
                  recording
                    ? 'trainer.stopRecording'
                    : processing
                      ? 'trainer.busy'
                      : waiting
                        ? 'trainer.allow'
                        : voice.hasPending
                          ? 'diagnostic.retrySubmit'
                          : 'trainer.start',
                )}
              </Button>
            )}
            {failed &&
              voice.error instanceof DiagnosticApiError &&
              [413, 415, 422].includes(voice.error.status) && (
                <Button variant="soft" onClick={voice.reset}>
                  {t('diagnostic.recordAgain')}
                </Button>
              )}
            <Text as="p" className={styles.microphoneStatus} aria-live="polite">
              {t(
                recording
                  ? 'session.recordingStatus'
                  : failed
                    ? 'session.errorStatus'
                    : processing
                      ? 'session.processingStatus'
                      : waiting
                        ? 'trainer.allow'
                        : 'session.microphoneReady',
              )}
            </Text>
          </aside>
        </>
      )}
    </main>
  )
}
function Question({
  task,
  transcript,
}: {
  task: DiagnosticTask
  transcript?: string
}) {
  const { t } = useTranslation()
  return (
    <section className={styles.question}>
      <div className={styles.questionToolbar}>
        <Text className={styles.questionBadge}>
          {t(`diagnostic.${task.role}`)}
        </Text>
      </div>
      <Text as="p" className={styles.eyebrow}>
        {task.competency_name}
      </Text>
      <Heading
        as="h1"
        className={
          task.question.length > 300
            ? diagnosticStyles.longQuestion
            : styles.questionTitle
        }
      >
        {task.question}
      </Heading>
      {task.options.length > 0 && (
        <section
          aria-labelledby="answer-options-title"
          className={styles.options}
        >
          <Heading as="h2" id="answer-options-title" className={styles.eyebrow}>
            {t('trainer.optionsTitle')}
          </Heading>
          <ol className={styles.optionList}>
            {task.options.map((option, index) => (
              <li key={index} className={styles.option}>
                <span className={styles.optionLetter} aria-hidden="true">
                  {index + 1}
                </span>
                <Text>{option}</Text>
              </li>
            ))}
          </ol>
        </section>
      )}
      <section
        className={styles.savedAnswer}
        aria-labelledby="transcript-title"
      >
        <Heading as="h2" id="transcript-title" className={styles.eyebrow}>
          {t('diagnostic.transcript')}
        </Heading>
        <Text as="p" className={styles.transcript}>
          {transcript ?? t('diagnostic.transcriptPending')}
        </Text>
      </section>
    </section>
  )
}
function Feedback({
  score,
  maxScore,
  verdict,
  feedback,
  criteria,
  heading = 'h2',
}: {
  score: number
  maxScore: number
  verdict: string
  feedback: string[]
  criteria?: DiagnosticProgress['criterion_results']
  heading?: 'h2' | 'h4'
}) {
  const { t } = useTranslation()
  return (
    <>
      <Text as="p" className={styles.eyebrow}>
        {t('session.result')}
      </Text>
      <div
        className={`${styles.score} ${score === 0 ? styles.scoreIncorrect : score < maxScore ? styles.scorePartial : ''}`}
      >
        <Text>
          {score} / {maxScore}
        </Text>
        <Text className={styles.verdict}>
          {t(`diagnostic.verdict.${verdict}`)}
        </Text>
      </div>
      <Heading as={heading} className={styles.feedbackTitle}>
        {t('trainer.feedback')}
      </Heading>
      {feedback.map((line, index) => (
        <Text as="p" key={index} className={styles.feedbackLine}>
          {line}
        </Text>
      ))}
      {criteria?.map((criterion) => (
        <Text as="p" className={styles.feedbackLine} key={criterion.key}>
          {t(
            criterion.satisfied
              ? 'diagnostic.criterionMet'
              : 'diagnostic.criterionUnmet',
          )}
          : {criterion.explanation}
        </Text>
      ))}
    </>
  )
}
function DiagnosticSummary({
  userId,
  sessionId,
  training,
}: {
  userId: string
  sessionId: string
  training: ReturnType<typeof useDiagnosticSession>
}) {
  const { t } = useTranslation()
  const result = useDiagnosticResultQuery(userId, sessionId)
  const { handleError } = training
  useEffect(() => {
    if (result.error) handleError(result.error)
  }, [result.error, handleError])
  if (result.isPending)
    return (
      <main className={styles.notice}>
        <Text role="status">{t('diagnostic.resultLoading')}</Text>
      </main>
    )
  if (result.isError)
    return (
      <main className={styles.notice}>
        <Text role="alert">
          {t(errorKey(result.error, 'diagnostic.resultError'))}
        </Text>
        {result.error instanceof DiagnosticApiError &&
        result.error.status === 404 ? (
          <Button
            disabled={training.restart.isPending}
            onClick={() => training.restart.mutate()}
          >
            {t(
              training.restart.isPending
                ? 'session.loading'
                : 'session.restart',
            )}
          </Button>
        ) : (
          <Button
            disabled={result.isFetching}
            onClick={() => void result.refetch()}
          >
            {t('trainer.retry')}
          </Button>
        )}
        {training.restart.isError && (
          <Text role="alert">{t('session.loadError')}</Text>
        )}
      </main>
    )
  const value = result.data
  return (
    <main className={styles.summary}>
      <Text as="p" className={styles.eyebrow}>
        {t('session.summary')}
      </Text>
      <Heading as="h1">{t('session.completed')}</Heading>
      <Text as="p" className={styles.summaryHelp}>
        {t('diagnostic.completedHelp')}
      </Text>
      <div className={styles.summaryScore}>
        <Text>
          {value.diagnostic_score} / {value.maximum_score}
        </Text>
        <Text>{t('diagnostic.totalScore')}</Text>
      </div>
      <Heading as="h2">{t('diagnostic.history')}</Heading>
      <ol className={diagnosticStyles.answers}>
        {value.answers.map((answer) => (
          <li key={answer.variant_task_id} className={diagnosticStyles.answer}>
            <Heading as="h3">{answer.task.question}</Heading>
            <Text as="p" className={styles.eyebrow}>
              {answer.task.competency_name} · {t(`diagnostic.${answer.role}`)}
            </Text>
            <Text as="p" className={styles.transcript}>
              {answer.text}
            </Text>
            <Feedback
              heading="h4"
              score={answer.score}
              maxScore={answer.grader_max_score}
              verdict={answer.verdict}
              feedback={answer.feedback}
              criteria={answer.criterion_results}
            />
          </li>
        ))}
      </ol>
      {value.untested_basics.length > 0 && (
        <section className={diagnosticStyles.untested}>
          <Heading as="h2">{t('diagnostic.untested')}</Heading>
          <ul>
            {value.untested_basics.map((task) => (
              <li key={task.variant_task_id}>{task.question}</li>
            ))}
          </ul>
        </section>
      )}
      {value.skipped_competencies.length > 0 && (
        <section className={diagnosticStyles.untested}>
          <Heading as="h2">{t('diagnostic.skippedCompetencies')}</Heading>
          <ul>
            {value.skipped_competencies.map((item) => (
              <li key={item.competency_id}>{item.competency_name}</li>
            ))}
          </ul>
        </section>
      )}
      {training.restart.isError && (
        <Text as="p" role="alert">
          {t('session.loadError')}
        </Text>
      )}
      <Button
        className={styles.primary}
        disabled={training.restart.isPending}
        onClick={() => training.restart.mutate()}
      >
        {t(training.restart.isPending ? 'session.loading' : 'session.restart')}
      </Button>
    </main>
  )
}
