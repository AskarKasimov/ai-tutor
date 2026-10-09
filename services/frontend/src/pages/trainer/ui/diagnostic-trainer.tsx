import { Button, Heading, Text } from '@radix-ui/themes'
import { LoaderCircle, Mic, RotateCcw, Square } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import {
  useDiagnosticResultQuery,
  useDiagnosticFeedbackQuery,
  useDiagnosticSession,
} from '@/features/diagnostic-session'
import { useDiagnosticVoice } from '@/features/diagnostic-session'
import type {
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/diagnostic-session'
import {
  TrainerAnswerPanel,
  TrainerCapturePanel,
  TrainerQuestion,
  TrainerScore,
  TrainerShell,
} from './trainer-shared'
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
  if (error.code === 'NO_SPEECH_DETECTED') return 'diagnostic.noSpeech'
  if (error.status === 415 || error.status === 422)
    return 'diagnostic.invalidAudio'
  if (error.code === 'INVALID_RESPONSE') return 'diagnostic.invalidResponse'
  return fallback
}
export function DiagnosticTrainer({
  subjectId,
  subjectName,
  onBack,
  initialSessionId,
  startNew = false,
}: {
  subjectId: string
  subjectName: string
  onBack: () => void
  initialSessionId?: string
  startNew?: boolean
}) {
  const { user } = useAuth()
  return user ? (
    <StudentDiagnostic
      key={`${user.id}:${subjectId}`}
      userId={user.id}
      subjectId={subjectId}
      subjectName={subjectName}
      onBack={onBack}
      initialSessionId={initialSessionId}
      startNew={startNew}
    />
  ) : null
}
function StudentDiagnostic({
  userId,
  subjectId,
  subjectName,
  onBack,
  initialSessionId,
  startNew,
}: {
  userId: string
  subjectId: string
  subjectName: string
  onBack: () => void
  initialSessionId?: string
  startNew: boolean
}) {
  const { t } = useTranslation()
  const training = useDiagnosticSession(
    userId,
    subjectId,
    initialSessionId,
    startNew,
  )
  const started = useRef(false)
  useEffect(() => {
    if (startNew && !started.current) {
      started.current = true
      training.restart.mutate()
    }
  }, [startNew, training.restart])
  const progress = training.query.data
  useEffect(() => {
    if (!progress) return
    try {
      const raw = sessionStorage.getItem('ai-tutor:learning-home')
      if (!raw) return
      const intent = JSON.parse(raw) as {
        userId?: string
        subjectId?: string
        sessionId?: string
      }
      if (
        intent.userId === userId &&
        intent.subjectId === subjectId &&
        !intent.sessionId
      ) {
        sessionStorage.setItem(
          'ai-tutor:learning-home',
          JSON.stringify({ ...intent, sessionId: progress.session_id }),
        )
      }
    } catch {
      /* A broken page intent must not affect the diagnostic session. */
    }
  }, [progress, subjectId, userId])
  return (
    <TrainerShell
      title={t('diagnostic.title')}
      subject={subjectName}
      progress={
        progress
          ? {
              label: t('diagnostic.progress', {
                completed: progress.completed_tasks,
                skipped: progress.skipped_tasks,
                total: progress.total_tasks,
              }),
              current: progress.completed_tasks + progress.skipped_tasks,
              total: progress.total_tasks,
            }
          : undefined
      }
    >
      <Button variant="soft" onClick={onBack}>
        {t('home.back')}
      </Button>
      {training.query.isPending || (startNew && training.restart.isPending) ? (
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
        </main>
      ) : progress ? (
        <DiagnosticFlow
          key={progress.session_id}
          userId={userId}
          onBack={onBack}
          training={training}
          progress={progress}
        />
      ) : null}
    </TrainerShell>
  )
}
function DiagnosticFlow({
  userId,
  onBack,
  training,
  progress,
}: {
  userId: string
  onBack: () => void
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
        onBack={onBack}
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
  const apiError =
    failed && voice.error instanceof DiagnosticApiError ? voice.error : null
  const needsRefresh =
    !!apiError &&
    ((apiError.status === 409 &&
      apiError.code !== 'DIAGNOSTIC_ANSWER_IN_PROGRESS') ||
      apiError.status === 404)
  // Rejected audio needs a fresh recording; technical failures also allow one.
  const rerecord = !!apiError && [413, 415, 422].includes(apiError.status)
  const canAlsoRerecord =
    failed &&
    voice.hasPending &&
    !rerecord &&
    (!apiError || apiError.status >= 500)
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
        <TrainerAnswerPanel>
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
        </TrainerAnswerPanel>
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
          <TrainerCapturePanel
            stage={voice.stage === 'result' ? 'ready' : voice.stage}
            seconds={seconds}
            stream={voice.stream}
            readyHelp="diagnostic.answerHelp"
            errorMessage={
              failed
                ? t(
                    voice.captureError
                      ? `trainer.${voice.captureError}`
                      : errorKey(voice.error, 'diagnostic.submitError'),
                  )
                : undefined
            }
            audioUrl={voice.audioUrl}
          >
            {needsRefresh ? (
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
                    : voice.hasPending && !rerecord
                      ? voice.retry()
                      : voice.start())
                }
              >
                {failed &&
                  (voice.hasPending && !rerecord ? (
                    <RotateCcw size={18} aria-hidden="true" />
                  ) : (
                    <Mic size={18} aria-hidden="true" />
                  ))}
                {t(
                  recording
                    ? 'trainer.stopRecording'
                    : processing
                      ? 'trainer.busy'
                      : waiting
                        ? 'trainer.allow'
                        : voice.hasPending && !rerecord
                          ? 'diagnostic.retrySubmit'
                          : failed
                            ? 'diagnostic.recordAgain'
                            : 'trainer.start',
                )}
              </Button>
            )}
            {canAlsoRerecord && (
              <Button
                variant="ghost"
                color="gray"
                className={styles.secondaryAction}
                onClick={() => void voice.start()}
              >
                {t('diagnostic.recordAgain')}
              </Button>
            )}
          </TrainerCapturePanel>
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
    <TrainerQuestion
      badge={t(`diagnostic.${task.role}`)}
      eyebrow={task.competency_name}
      question={task.question}
      options={task.options}
    >
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
    </TrainerQuestion>
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
      <TrainerScore
        score={score}
        maxScore={maxScore}
        verdict={t(`diagnostic.verdict.${verdict}`)}
      />
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
  onBack,
}: {
  userId: string
  sessionId: string
  training: ReturnType<typeof useDiagnosticSession>
  onBack: () => void
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
      <DiagnosticOverallFeedback userId={userId} sessionId={sessionId} />
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
      <Button className={styles.primary} onClick={onBack}>
        {t('home.newDiagnostic')}
      </Button>
    </main>
  )
}

function DiagnosticOverallFeedback({
  userId,
  sessionId,
}: {
  userId: string
  sessionId: string
}) {
  const { t } = useTranslation()
  const feedback = useDiagnosticFeedbackQuery(userId, sessionId)
  const value = feedback.data
  return (
    <section
      className={diagnosticStyles.overallFeedback}
      aria-label={t('diagnostic.overallFeedback')}
    >
      <Heading as="h2">{t('diagnostic.overallFeedback')}</Heading>
      {feedback.isPending ? (
        <Text as="p" role="status">
          {t('diagnostic.feedbackLoading')}
        </Text>
      ) : feedback.isError ? (
        <div>
          <Text as="p" role="alert">
            {t('diagnostic.feedbackError')}
          </Text>
          <Button
            onClick={() => void feedback.refetch()}
            disabled={feedback.isFetching}
          >
            {t('diagnostic.feedbackRetry')}
          </Button>
        </div>
      ) : value ? (
        <>
          <Text as="p" className={diagnosticStyles.overallSummary}>
            {value.summary}
          </Text>
          {value.strengths.length > 0 && (
            <div>
              <Heading as="h3">{t('diagnostic.strengths')}</Heading>
              <ul>
                {value.strengths.map((item, index) => (
                  <li key={index}>{item}</li>
                ))}
              </ul>
            </div>
          )}
          {value.partial_competencies.length > 0 && (
            <div>
              <Heading as="h3">{t('diagnostic.partialCompetencies')}</Heading>
              <ul>
                {value.partial_competencies.map((item) => (
                  <li key={item.competency_id}>
                    {item.competency_name}: {item.details}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {value.confirmed_gaps.length > 0 && (
            <div>
              <Heading as="h3">{t('diagnostic.confirmedGaps')}</Heading>
              <ul>
                {value.confirmed_gaps.map((item) => (
                  <li key={item.outcome_id}>
                    {item.competency_name} — {item.outcome_name}: {item.advice}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {value.training_recommendations.length > 0 && (
            <div>
              <Heading as="h3">{t('diagnostic.recommendations')}</Heading>
              <ol>
                {value.training_recommendations.map((item) => (
                  <li key={item.outcome_id}>
                    {item.competency_name} — {item.outcome_name}:{' '}
                    {item.rationale}
                  </li>
                ))}
              </ol>
            </div>
          )}
          {value.unverified_competencies.length > 0 && (
            <div>
              <Heading as="h3">
                {t('diagnostic.unverifiedCompetencies')}
              </Heading>
              <ul>
                {value.unverified_competencies.map((item) => (
                  <li key={item.competency_id}>{item.competency_name}</li>
                ))}
              </ul>
            </div>
          )}
        </>
      ) : null}
    </section>
  )
}
