import { MarkdownContent } from '@/shared/ui'
import { ResultReport, reportSummary } from '@/widgets/result-report'
import { Button, Heading, Text } from '@radix-ui/themes'
import { ChevronRight, Mic, RotateCcw, Zap } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import { DiagnosticApiError } from '@/entities/learning-diagnostic-session'
import {
  useReviewAudioPlayer,
  useDiagnosticResultQuery,
  useDiagnosticFeedbackQuery,
  useDiagnosticSession,
} from '@/features/learning-diagnostic-session'
import { useDiagnosticVoice } from '@/features/learning-diagnostic-session'
import type {
  DiagnosticProgress,
  DiagnosticTask,
} from '@/entities/learning-diagnostic-session'
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
import { toResultReport } from '../model/to-result-report'
import { ConfirmVoiceAnswer } from './trainer-answer'
import styles from '@/pages/learning-trainer/ui/trainer-layout.module.scss'
import diagnosticStyles from '@/pages/learning-trainer/ui/diagnostic.module.scss'

function errorKey(error: unknown, fallback: string) {
  if (!(error instanceof DiagnosticApiError)) return fallback
  if (error.code === 'DIAGNOSTIC_ANSWER_IN_PROGRESS')
    return 'learningDiagnostic.answerInProgress'
  if (error.code === 'NO_ELIGIBLE_COMPETENCIES')
    return 'learningDiagnostic.unavailable'
  if (error.status === 401) return 'learningTrainer.unauthorized'
  if (error.status === 404) return 'learningDiagnostic.expired'
  if (error.status === 409) return 'learningDiagnostic.conflict'
  if (error.status === 413) return 'learningDiagnostic.recordingLimit'
  if (error.code === 'NO_SPEECH_DETECTED') return 'learningDiagnostic.noSpeech'
  if (error.status === 415 || error.status === 422)
    return 'learningDiagnostic.invalidAudio'
  if (error.code === 'INVALID_RESPONSE')
    return 'learningDiagnostic.invalidResponse'
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
      title={t('learningDiagnostic.title')}
      subject={subjectName}
      onBack={onBack}
      progress={
        progress ? (
          <TrainerProgress
            label={t('learningSession.diagnosticProgress')}
            percent={diagnosticPercent(progress)}
          />
        ) : undefined
      }
    >
      {training.query.isPending || (startNew && training.restart.isPending) ? (
        <main className={styles.notice}>
          <Text role="status">{t('learningSession.loading')}</Text>
        </main>
      ) : training.query.isError ? (
        <main className={styles.notice}>
          <Text role="alert">
            {t(
              errorKey(
                training.query.error,
                training.query.error instanceof DiagnosticApiError &&
                  training.query.error.status === 422
                  ? 'learningDiagnostic.unavailable'
                  : 'learningSession.loadError',
              ),
            )}
          </Text>
          {training.query.error instanceof DiagnosticApiError &&
          training.query.error.status === 404 ? (
            <Button
              disabled={training.restart.isPending}
              onClick={() => training.restart.mutate()}
            >
              {t('learningSession.restart')}
            </Button>
          ) : (
            <Button
              disabled={training.query.isFetching}
              onClick={() => void training.query.refetch()}
            >
              {t('learningTrainer.retry')}
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
        <Text>{t('learningSession.empty')}</Text>
      </main>
    )
  const recording = voice.stage === 'recording'
  const processing =
    voice.stage === 'processing' || voice.stage === 'recognizing'
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
          title: t('learningTrainer.micFailedTitle'),
          help: t('learningTrainer.micFailedHelp'),
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
          {t('learningTrainer.speechError')}
        </Text>
      )}
      {!response && voice.cancelledHint && (
        <Text as="p" role="status" className={sessionStyles.hint}>
          {t('learningTrainer.audioCancelled')}
        </Text>
      )}
    </>
  )
  return (
    <TrainerWorkspace>
      <Question
        task={task}
        transcript={response?.text ?? voice.previewText}
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
                ? 'learningSession.viewSummary'
                : 'learningSession.next',
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
                    ? `learningTrainer.${voice.captureError}`
                    : errorKey(voice.error, 'learningDiagnostic.submitError'),
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
              {t('learningDiagnostic.refresh')}
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
                  ? voice.hasConfirmed
                    ? 'learningDiagnostic.retrySubmit'
                    : 'learningTrainer.retryRecognition'
                  : micError
                    ? 'learningTrainer.retry'
                    : 'learningDiagnostic.recordAgain',
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
              {t('learningDiagnostic.recordAgain')}
            </Button>
          )}
          {voice.stage === 'review' && (
            <ConfirmVoiceAnswer
              onSend={() => void voice.retry()}
              onRerecord={() => void voice.start()}
            />
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
      badge={t(`learningDiagnostic.${task.role}`)}
      question={task.question}
      options={task.options}
      instruction={instruction}
    >
      <TrainerTranscript id="transcript-title" text={transcript} autoReveal />
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
        {t('learningSession.result')}
      </Text>
      <div className={diagnosticStyles.reviewScore}>
        <TrainerScore
          score={score}
          maxScore={maxScore}
          verdict={t(
            `learningDiagnostic.verdict.${skipped ? 'skipped' : verdict}`,
          )}
          skipped={skipped}
        />
      </div>
      <Heading as={heading} className={styles.feedbackTitle}>
        {t('learningTrainer.feedback')}
      </Heading>
      <MarkdownContent
        text={feedback.join(' ')}
        className={styles.feedbackLine}
      />
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
  const reviewAudio = useReviewAudioPlayer()
  const { handleError } = training
  useEffect(() => {
    if (result.error) handleError(result.error)
  }, [result.error, handleError])
  if (result.isPending)
    return (
      <main className={styles.notice}>
        <Text role="status">{t('learningDiagnostic.resultLoading')}</Text>
      </main>
    )
  if (result.isError)
    return (
      <main className={styles.notice}>
        <Text role="alert">
          {t(errorKey(result.error, 'learningDiagnostic.resultError'))}
        </Text>
        {result.error instanceof DiagnosticApiError &&
        result.error.status === 404 ? (
          <Button
            disabled={training.restart.isPending}
            onClick={() => training.restart.mutate()}
          >
            {t(
              training.restart.isPending
                ? 'learningSession.loading'
                : 'learningSession.restart',
            )}
          </Button>
        ) : (
          <Button
            disabled={result.isFetching}
            onClick={() => void result.refetch()}
          >
            {t('learningTrainer.retry')}
          </Button>
        )}
        {training.restart.isError && (
          <Text role="alert">{t('learningSession.loadError')}</Text>
        )}
      </main>
    )
  const value = result.data
  const hasGaps = value.answers.some(
    (answer) => answer.score < (answer.role === 'basic' ? 1 : 2),
  )
  // The model writes only the general paragraph; headings of older texts are
  // cut off.
  const summary = reportSummary(
    overall,
    (data) =>
      parseStructuredSummary(data.summary || '').generalText ||
      normalizeText(data.summary || ''),
  )
  return (
    <main className={styles.summary}>
      <ResultReport
        report={toResultReport(value)}
        summary={summary}
        highlight={
          hasGaps ? (
            <div className={diagnosticStyles.nextStep}>
              <Text as="p" className={diagnosticStyles.nextStepText}>
                {t('learningDiagnostic.nextStep')}
              </Text>
              <Button size="3" onClick={() => onTraining(sessionId)}>
                <Zap size={18} aria-hidden="true" />
                {t('learningDiagnostic.toTraining')}
              </Button>
            </div>
          ) : undefined
        }
      >
        {!overall.isPending && (
          <>
            <button
              type="button"
              className={diagnosticStyles.reviewToggle}
              aria-expanded={reviewOpen}
              aria-controls="task-review"
              onClick={() =>
                setReviewOpen((open) => {
                  if (open) reviewAudio.stop()
                  return !open
                })
              }
            >
              <ChevronRight
                size={20}
                className={diagnosticStyles.reviewChevron}
                aria-hidden="true"
              />
              {t('learningDiagnostic.reviewToggle', {
                count: value.answers.length,
              })}
            </button>
            {reviewOpen && (
              <div id="task-review">
                <Heading as="h2" className={diagnosticStyles.visuallyHidden}>
                  {t('learningDiagnostic.history')}
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
                      {answer.task.source_task_id && (
                        <div className={diagnosticStyles.reviewAudio}>
                          <ListenButton
                            state={
                              reviewAudio.loadingId ===
                              answer.task.source_task_id
                                ? 'loading'
                                : reviewAudio.playingId ===
                                    answer.task.source_task_id
                                  ? 'playing'
                                  : 'idle'
                            }
                            onClick={() =>
                              void reviewAudio.play(answer.task.source_task_id)
                            }
                          />
                          {reviewAudio.notice?.taskId ===
                            answer.task.source_task_id && (
                            <>
                              <Text as="p" className={sessionStyles.hint}>
                                {answer.task.voice_instruction}
                              </Text>
                              <Text
                                as="p"
                                role={
                                  reviewAudio.notice.key ===
                                  'learningTrainer.speechError'
                                    ? 'alert'
                                    : 'status'
                                }
                                className={sessionStyles.hint}
                              >
                                {t(reviewAudio.notice.key)}
                              </Text>
                            </>
                          )}
                        </div>
                      )}
                      <div className={diagnosticStyles.reviewAnswerSection}>
                        <Text as="p" className={styles.eyebrow}>
                          {t('learningTrainer.yourAnswer')}
                        </Text>
                        <Text
                          as="p"
                          className={diagnosticStyles.reviewAnswerText}
                        >
                          {answer.text}
                        </Text>
                      </div>
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
      </ResultReport>
      {training.restart.isError && (
        <Text as="p" role="alert">
          {t('learningSession.loadError')}
        </Text>
      )}
    </main>
  )
}
