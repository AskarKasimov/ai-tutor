import { Button, Heading, Text } from '@radix-ui/themes'
import {
  AlertTriangle,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  Mic,
  RotateCcw,
  Sparkles,
  Zap,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
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
  ListenButton,
  TrainerCapturePanel,
  TrainerProgress,
  TrainerQuestion,
  TrainerScore,
  TrainerShell,
  TrainerTranscript,
  TrainerWorkspace,
  sessionStyles,
  FormattedQuestionContent,
} from './trainer-shared'
import { normalizeText, parseStructuredSummary } from '@/entities/assessment'
import { diagnosticPercent } from '../model/diagnostic-percent'
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
  onTraining,
  initialSessionId,
  startNew = false,
}: {
  subjectId: string
  subjectName: string
  onBack: () => void
  onTraining: (diagnosticId: string) => void
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
      onTraining={onTraining}
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
  onTraining,
  initialSessionId,
  startNew,
}: {
  userId: string
  subjectId: string
  subjectName: string
  onBack: () => void
  onTraining: (diagnosticId: string) => void
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
      onBack={onBack}
      progress={
        progress ? (
          <TrainerProgress
            label={t('session.diagnosticProgress')}
            percent={diagnosticPercent(progress)}
          />
        ) : undefined
      }
    >
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
          onTraining={onTraining}
          training={training}
          progress={progress}
        />
      ) : null}
    </TrainerShell>
  )
}
function DiagnosticFlow({
  userId,
  onTraining,
  training,
  progress,
}: {
  userId: string
  onTraining: (diagnosticId: string) => void
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
  const openTaskId = voice.accepted
    ? undefined
    : progress.current?.variant_task_id
  // Read the stored voice instruction aloud whenever a new task opens.
  useEffect(() => {
    if (openTaskId) void voice.speak({ auto: true })
    // voice is recreated on every render; autoplay runs once per opened task.
  }, [openTaskId])
  if (progress.status === 'completed' && !voice.accepted)
    return (
      <DiagnosticSummary
        userId={userId}
        sessionId={progress.session_id}
        training={training}
        onTraining={onTraining}
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
  const micError =
    failed &&
    (voice.captureError === 'denied' || voice.captureError === 'unavailable')
      ? {
          title: t('trainer.micFailedTitle'),
          help: t('trainer.micFailedHelp'),
        }
      : undefined
  const instruction = (
    <>
      <ListenButton
        state={voice.speech}
        disabled={!response && blocked}
        onClick={() => void voice.speak()}
      />
      {voice.speechError && (
        <Text as="p" role="alert" className={sessionStyles.hint}>
          {t('trainer.speechError')}
        </Text>
      )}
      {!response && voice.cancelledHint && (
        <Text as="p" role="status" className={sessionStyles.hint}>
          {t('trainer.audioCancelled')}
        </Text>
      )}
    </>
  )
  return (
    <TrainerWorkspace>
      <Question
        task={task}
        transcript={response?.text}
        instruction={instruction}
      />
      {response ? (
        <TrainerCapturePanel
          stage="ready"
          seconds={seconds}
          answered={response.answer_skipped ? 'skipped' : 'accepted'}
        >
          <Button className={sessionStyles.primary} onClick={voice.reset}>
            {t(
              progress.status === 'completed'
                ? 'session.viewSummary'
                : 'session.next',
            )}
          </Button>
        </TrainerCapturePanel>
      ) : (
        <TrainerCapturePanel
          stage={voice.stage === 'result' ? 'ready' : voice.stage}
          seconds={seconds}
          stream={voice.stream}
          errorMessage={
            failed
              ? t(
                  voice.captureError
                    ? `trainer.${voice.captureError}`
                    : errorKey(voice.error, 'diagnostic.submitError'),
                )
              : undefined
          }
          micError={micError}
          onMic={
            // The circle only starts or stops a recording; a pending retry or
            // a stale question keeps their explicit buttons.
            needsRefresh ||
            processing ||
            waiting ||
            (!recording && voice.hasPending && !rerecord)
              ? undefined
              : () => void (recording ? voice.stop() : voice.start())
          }
        >
          {needsRefresh ? (
            <Button
              className={sessionStyles.primary}
              onClick={() => void refresh()}
            >
              {t('diagnostic.refresh')}
            </Button>
          ) : failed ? (
            // Recording starts and stops from the microphone circle; the
            // button only recovers from a failure.
            <Button
              className={sessionStyles.primary}
              onClick={() =>
                void (voice.hasPending && !rerecord
                  ? voice.retry()
                  : voice.start())
              }
            >
              {voice.hasPending && !rerecord ? (
                <RotateCcw size={18} aria-hidden="true" />
              ) : (
                <Mic size={18} aria-hidden="true" />
              )}
              {t(
                voice.hasPending && !rerecord
                  ? 'diagnostic.retrySubmit'
                  : micError
                    ? 'trainer.retry'
                    : 'diagnostic.recordAgain',
              )}
            </Button>
          ) : null}
          {canAlsoRerecord && (
            <Button
              variant="ghost"
              color="gray"
              className={sessionStyles.secondary}
              onClick={() => void voice.start()}
            >
              {t('diagnostic.recordAgain')}
            </Button>
          )}
        </TrainerCapturePanel>
      )}
    </TrainerWorkspace>
  )
}
function Question({
  task,
  transcript,
  instruction,
}: {
  task: DiagnosticTask
  transcript?: string
  instruction?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <TrainerQuestion
      badge={t(`diagnostic.${task.role}`)}
      question={task.question}
      options={task.options}
      instruction={instruction}
    >
      <TrainerTranscript id="transcript-title" text={transcript} />
    </TrainerQuestion>
  )
}
function Feedback({
  score,
  maxScore,
  verdict,
  skipped,
  feedback,
  heading = 'h2',
}: {
  score: number
  maxScore: number
  verdict: string
  skipped?: boolean
  feedback: string[]
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
        verdict={t(`diagnostic.verdict.${skipped ? 'skipped' : verdict}`)}
        skipped={skipped}
      />
      <Heading as={heading} className={styles.feedbackTitle}>
        {t('trainer.feedback')}
      </Heading>
      <Text as="p" className={styles.feedbackLine}>
        {feedback.join(' ')}
      </Text>
    </>
  )
}
function DiagnosticSummary({
  userId,
  sessionId,
  training,
  onTraining,
}: {
  userId: string
  sessionId: string
  training: ReturnType<typeof useDiagnosticSession>
  onTraining: (diagnosticId: string) => void
}) {
  const { t } = useTranslation()
  const result = useDiagnosticResultQuery(userId, sessionId)
  // The per-task review appears together with the overall feedback.
  const overall = useDiagnosticFeedbackQuery(userId, sessionId)
  // The task review is long; it stays folded so training is one click away.
  const [reviewOpen, setReviewOpen] = useState(false)
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
      {/* The result leads straight on: a bright card whose main action is
          to work through the weak topics. */}
      <div className={diagnosticStyles.scoreHero}>
        <div className={diagnosticStyles.scoreText}>
          <div>
            <Text className={diagnosticStyles.scoreNumber}>
              {value.diagnostic_score} / {value.maximum_score}
            </Text>{' '}
            <Text className={diagnosticStyles.scoreLabel}>
              {t('diagnostic.totalScore')}
            </Text>
          </div>
          <Text as="p" className={diagnosticStyles.scoreNext}>
            {t('diagnostic.nextStep')}
          </Text>
        </div>
        <button
          type="button"
          className={diagnosticStyles.toTraining}
          onClick={() => onTraining(sessionId)}
        >
          <span className={diagnosticStyles.toTrainingIcon} aria-hidden="true">
            <Zap size={20} fill="currentColor" />
          </span>
          {t('diagnostic.toTraining')}
        </button>
      </div>
      {overall.isPending ? (
        <FeedbackPreparing />
      ) : (
        <DiagnosticOverallFeedback feedback={overall} />
      )}
      {!overall.isPending && (
        <>
          <button
            type="button"
            className={diagnosticStyles.reviewToggle}
            aria-expanded={reviewOpen}
            aria-controls="task-review"
            onClick={() => setReviewOpen((open) => !open)}
          >
            <ChevronRight
              size={20}
              className={diagnosticStyles.reviewChevron}
              aria-hidden="true"
            />
            {t('diagnostic.reviewToggle', { count: value.answers.length })}
          </button>
          {reviewOpen && (
            <div id="task-review">
              <Heading as="h2" className={diagnosticStyles.visuallyHidden}>
                {t('diagnostic.history')}
              </Heading>
              <ol className={diagnosticStyles.answers}>
                {value.answers.map((answer) => (
                  <li
                    key={answer.variant_task_id}
                    className={diagnosticStyles.answer}
                  >
                    <FormattedQuestionContent
                      question={answer.task.question}
                      headingLevel="h3"
                    />
                    <Text as="p" className={styles.eyebrow}>
                      {answer.task.competency_name} ·{' '}
                      {t(`diagnostic.${answer.role}`)}
                    </Text>
                    <Text as="p" className={styles.transcript}>
                      {answer.text}
                    </Text>
                    <Feedback
                      heading="h4"
                      score={answer.score}
                      maxScore={answer.grader_max_score}
                      verdict={answer.verdict}
                      skipped={answer.skipped}
                      feedback={answer.feedback}
                    />
                  </li>
                ))}
              </ol>
              {/* Basics left out after a full main answer and competencies
                  outside the variant stay in the API: the review shows only
                  the tasks that were asked. */}
            </div>
          )}
        </>
      )}
      {training.restart.isError && (
        <Text as="p" role="alert">
          {t('session.loadError')}
        </Text>
      )}
    </main>
  )
}

// Shown while the model writes the overall feedback; the task review waits too.
function FeedbackPreparing() {
  const { t } = useTranslation()
  return (
    <section className={diagnosticStyles.preparing} role="status">
      <span className={diagnosticStyles.preparingIcon} aria-hidden="true">
        <Sparkles size={28} />
      </span>
      <div className={diagnosticStyles.preparingText}>
        <Heading as="h2">{t('diagnostic.feedbackLoading')}</Heading>
        <Text as="p">{t('diagnostic.feedbackLoadingHelp')}</Text>
      </div>
      <div className={diagnosticStyles.skeleton} aria-hidden="true">
        <span />
        <span />
        <span />
      </div>
    </section>
  )
}

function DiagnosticOverallFeedback({
  feedback,
}: {
  feedback: ReturnType<typeof useDiagnosticFeedbackQuery>
}) {
  const { t } = useTranslation()
  const value = feedback.data
  return (
    <section
      className={diagnosticStyles.overallFeedback}
      aria-label={t('diagnostic.overallFeedback')}
    >
      <div className={styles.feedbackHeaderTitle}>
        <Sparkles size={20} className={styles.sparkleIcon} aria-hidden="true" />
        <Heading as="h2">{t('diagnostic.overallFeedback')}</Heading>
      </div>

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
        <OverallFeedbackContent value={value} />
      ) : null}
    </section>
  )
}

type OverallFeedback = NonNullable<
  ReturnType<typeof useDiagnosticFeedbackQuery>['data']
>

// The model summary splits into a general paragraph, "what is good" and "what
// to work on". Cards are filled from the report when the model leaves a part
// out and are hidden when there is nothing to say: no invented praise.
function OverallFeedbackContent({ value }: { value: OverallFeedback }) {
  const { t } = useTranslation()
  const structured = parseStructuredSummary(value.summary || '')
  const generalText =
    structured.generalText || normalizeText(value.summary || '')
  // What to work on next: the recommendations in their order, then confirmed
  // gaps they do not cover, each with its own advice.
  const covered = new Set(
    value.training_recommendations.map((item) => item.outcome_id),
  )
  const recommendations = [
    ...value.training_recommendations.map((item) => ({
      id: item.outcome_id,
      title: `${item.competency_name} — ${item.outcome_name}`,
      text: item.rationale,
    })),
    ...value.confirmed_gaps
      .filter((gap) => !covered.has(gap.outcome_id))
      .map((gap) => ({
        id: gap.outcome_id,
        title: `${gap.competency_name} — ${gap.outcome_name}`,
        text: gap.advice,
      })),
  ]
  const good = structured.whatIsGood ? (
    <p className={styles.feedbackParagraph}>{structured.whatIsGood}</p>
  ) : value.strengths.length > 0 ? (
    <ul className={styles.summaryCardList}>
      {value.strengths.map((item, index) => (
        <li key={index}>{normalizeText(item)}</li>
      ))}
    </ul>
  ) : null
  const work = structured.whatNeedsWork ? (
    <p className={styles.feedbackParagraph}>{structured.whatNeedsWork}</p>
  ) : value.partial_competencies.length > 0 ? (
    <ul className={styles.summaryCardList}>
      {value.partial_competencies.map((part) => (
        <li key={part.competency_id}>
          <span>
            <strong>{part.competency_name}:</strong> {part.details}
          </span>
        </li>
      ))}
    </ul>
  ) : null
  return (
    <div className={styles.feedbackContent}>
      {generalText && (
        <div className={styles.feedbackSummaryText}>
          {generalText
            .split(/\n+/)
            .filter((paragraph) => paragraph.trim())
            .map((paragraph, index) => (
              <p key={index} className={styles.feedbackParagraph}>
                {paragraph.trim()}
              </p>
            ))}
        </div>
      )}
      {(good || work) && (
        <div className={styles.summaryCardsGrid}>
          {good && (
            <section
              className={`${styles.summaryCard} ${styles.summaryCardGood}`}
            >
              <h3 className={styles.summaryCardHeader}>
                <CheckCircle2
                  size={18}
                  className={styles.iconSuccess}
                  aria-hidden="true"
                />
                <span className={styles.summaryCardTitleGood}>
                  {t('diagnostic.feedbackGood')}
                </span>
              </h3>
              <div className={styles.summaryCardBody}>{good}</div>
            </section>
          )}
          {work && (
            <section
              className={`${styles.summaryCard} ${styles.summaryCardWarning}`}
            >
              <h3 className={styles.summaryCardHeader}>
                <AlertTriangle
                  size={18}
                  className={styles.iconWarning}
                  aria-hidden="true"
                />
                <span className={styles.summaryCardTitleWarning}>
                  {t('diagnostic.feedbackWork')}
                </span>
              </h3>
              <div className={styles.summaryCardBody}>{work}</div>
            </section>
          )}
        </div>
      )}
      {recommendations.length > 0 && (
        <section className={styles.recommendationsCard}>
          <h3 className={styles.recommendationsHeader}>
            <BookOpen size={18} className={styles.iconRec} aria-hidden="true" />
            <span className={styles.recommendationsTitle}>
              {t('diagnostic.recommendations')}
            </span>
          </h3>
          <ol className={styles.recommendationsList}>
            {recommendations.map((item, index) => (
              <li key={item.id}>
                <span className={styles.recNumber} aria-hidden="true">
                  {index + 1}
                </span>
                <span>
                  <strong>{item.title}:</strong> {item.text}
                </span>
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  )
}
