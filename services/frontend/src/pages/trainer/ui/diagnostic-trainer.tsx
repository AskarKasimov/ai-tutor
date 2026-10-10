import { Button, Heading, Text } from '@radix-ui/themes'
import {
  ArrowRight,
  ChevronRight,
  Mic,
  RotateCcw,
  Sparkles,
  Square,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
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
  QuestionNavigator,
  TrainerAnswerPanel,
  TrainerCapturePanel,
  TrainerQuestion,
  TrainerScore,
  TrainerShell,
  TrainerTranscript,
  TrainerWorkspace,
  sessionStyles,
} from './trainer-shared'
import type { NavigatorItem, NavigatorState } from './trainer-shared'
import {
  initialTrack,
  readTopicTrack,
  readTrack,
  recordDiagnosticAnswer,
  recordTopicAnswer,
  scoreState,
  writeTopicTrack,
  writeTrack,
} from '../model/question-track'
import type {
  QuestionTrack,
  TopicTrack,
  TrackState,
} from '../model/question-track'
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
  const track = useDiagnosticTrack(progress)
  const crumb =
    track.kind === 'topic'
      ? track.crumb &&
        t(
          track.crumb.basic ? 'session.topicBasicCrumb' : 'session.topicCrumb',
          {
            number: track.crumb.topic,
            count: track.crumb.count,
          },
        )
      : track.question !== undefined
        ? t('session.questionNumber', { number: track.question })
        : undefined
  return (
    <TrainerShell
      title={t('diagnostic.title')}
      subject={subjectName}
      crumb={crumb}
      onBack={onBack}
      navigator={
        progress ? (
          <QuestionNavigator
            label={t('session.navigatorLabel')}
            items={track.items}
            progress={track.progress}
            kind={track.kind}
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
          onAnswered={track.record}
          onShown={track.clearShown}
          training={training}
          progress={progress}
        />
      ) : null}
    </TrainerShell>
  )
}
// Navigator states for the fixed diagnostic order: answered positions keep
// the colors recorded in this browser, the next position is current.
type TopicPosition = { topic: number; step: number }

function trackState(state: TrackState | undefined): NavigatorState {
  return state === 'correct' ||
    state === 'partial' ||
    state === 'incorrect' ||
    state === 'skipped'
    ? state
    : 'done'
}

// Navigator states. With competency_count the navigator shows topics: their
// number is fixed, while clarifying (basic) questions appear only when asked.
// Older sessions without it keep the per-question view.
function useDiagnosticTrack(progress?: DiagnosticProgress) {
  const sessionId = progress?.session_id
  const done = progress ? progress.completed_tasks + progress.skipped_tasks : 0
  const [, rerender] = useState(0)
  const [shown, setShownIndex] = useState<number | null>(null)
  const [shownTopic, setShownTopic] = useState<TopicPosition | null>(null)
  const current = useRef<{
    id?: string
    track: QuestionTrack | null
    topics: TopicTrack
  }>({ track: null, topics: {} })
  if (sessionId && current.current.id !== sessionId)
    current.current = {
      id: sessionId,
      track:
        readTrack(sessionId) ??
        initialTrack(done, progress?.skipped_tasks ?? 0),
      topics: readTopicTrack(sessionId),
    }
  // Remember where each shown task sits, so its answer lands in its topic.
  const positions = useRef(new Map<string, TopicPosition>())
  if (progress?.current && progress.current_competency)
    positions.current.set(progress.current.variant_task_id, {
      topic: progress.current_competency,
      step: progress.current_step ?? 0,
    })
  const handled = useRef<unknown>(null)
  const record = useCallback((taskId: string, response: DiagnosticProgress) => {
    const id = current.current.id
    const base = current.current.track
    if (!id || !base || handled.current === response) return
    handled.current = response
    const next = recordDiagnosticAnswer(base, response)
    let topics = current.current.topics
    const position = positions.current.get(taskId)
    if (position) {
      topics = recordTopicAnswer(
        topics,
        position.topic,
        position.step,
        response.answer_skipped
          ? 'skipped'
          : scoreState(response.score ?? 0, response.grader_max_score ?? 1),
      )
      writeTopicTrack(id, topics)
    }
    current.current = { id, track: next.track, topics }
    writeTrack(id, next.track)
    setShownIndex(next.index)
    setShownTopic(position ?? null)
    rerender((value) => value + 1)
  }, [])
  const clearShown = useCallback(() => {
    setShownIndex(null)
    setShownTopic(null)
  }, [])
  const count = progress?.competency_count
  if (progress && count) {
    const active = progress.status === 'active'
    const topic = active ? (progress.current_competency ?? 1) : count + 1
    const step = active ? (progress.current_step ?? 0) : 0
    const items: NavigatorItem[] = Array.from({ length: count }, (_, i) => {
      const number = i + 1
      const states = current.current.topics[number] ?? []
      const basics: NavigatorState[] = []
      for (const basic of [1, 2]) {
        if (states[basic]) basics.push(trackState(states[basic]))
        else if (number === topic && basic === step) basics.push('current')
      }
      return {
        number,
        state: states[0]
          ? trackState(states[0])
          : number < topic || (number === topic && step > 0)
            ? 'done'
            : number === topic
              ? 'current'
              : 'locked',
        basics,
      }
    })
    const position = shownTopic ?? (active ? { topic, step } : null)
    return {
      kind: 'topic' as const,
      items,
      record,
      clearShown,
      progress: { current: Math.min(topic - 1, count), total: count },
      crumb: position
        ? { topic: position.topic, count, basic: position.step > 0 }
        : undefined,
    }
  }
  const states = current.current.track?.states ?? []
  const items: NavigatorItem[] = progress
    ? Array.from({ length: progress.total_tasks }, (_, index) => ({
        number: index + 1,
        state:
          index < done
            ? trackState(states[index])
            : index === done && progress.status === 'active'
              ? 'current'
              : 'locked',
      }))
    : []
  const number =
    shown !== null
      ? shown + 1
      : progress?.status === 'active'
        ? done + 1
        : undefined
  return {
    kind: 'question' as const,
    items,
    record,
    clearShown,
    progress: progress
      ? { current: done, total: progress.total_tasks }
      : undefined,
    question: number,
  }
}

function DiagnosticFlow({
  userId,
  onTraining,
  onAnswered,
  onShown,
  training,
  progress,
}: {
  userId: string
  onTraining: (diagnosticId: string) => void
  onAnswered: (taskId: string, response: DiagnosticProgress) => void
  onShown: () => void
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
  const accepted = voice.accepted
  useEffect(() => {
    if (accepted) onAnswered(accepted.task.variant_task_id, accepted.progress)
    else onShown()
  }, [accepted, onAnswered, onShown])
  const [skipFailed, setSkipFailed] = useState(false)
  async function skip() {
    setSkipFailed(false)
    try {
      await voice.skip()
    } catch {
      setSkipFailed(true)
    }
  }
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
  return (
    <TrainerWorkspace>
      <Question task={task} transcript={response?.text} />
      {response ? (
        <TrainerAnswerPanel>
          <Feedback
            score={response.score!}
            maxScore={response.grader_max_score!}
            verdict={response.verdict!}
            skipped={response.answer_skipped}
            feedback={response.feedback!}
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
          <div className={sessionStyles.actions}>
            <Button className={sessionStyles.primary} onClick={voice.reset}>
              {t(
                progress.status === 'completed'
                  ? 'session.viewSummary'
                  : 'session.next',
              )}
            </Button>
          </div>
        </TrainerAnswerPanel>
      ) : (
        <TrainerCapturePanel
          stage={voice.stage === 'result' ? 'ready' : voice.stage}
          seconds={seconds}
          stream={voice.stream}
          speech={
            <>
              <ListenButton
                state={voice.speech}
                disabled={blocked}
                onClick={() => void voice.speak()}
              />
              {voice.speechError && (
                <Text as="p" role="alert" className={sessionStyles.hint}>
                  {t('trainer.speechError')}
                </Text>
              )}
              {voice.pendingHint && (
                <Text as="p" role="status" className={sessionStyles.hint}>
                  {t('trainer.audioPreparing')}
                </Text>
              )}
              {voice.cancelledHint && (
                <Text as="p" role="status" className={sessionStyles.hint}>
                  {t('trainer.audioCancelled')}
                </Text>
              )}
            </>
          }
          errorMessage={
            skipFailed
              ? t('session.skipError')
              : failed
                ? t(
                    voice.captureError
                      ? `trainer.${voice.captureError}`
                      : errorKey(voice.error, 'diagnostic.submitError'),
                  )
                : undefined
          }
          micError={micError}
          audioUrl={voice.audioUrl}
          onSkip={() => void skip()}
          skipDisabled={blocked}
        >
          {needsRefresh ? (
            <Button
              className={sessionStyles.primary}
              onClick={() => void refresh()}
            >
              {t('diagnostic.refresh')}
            </Button>
          ) : (
            <Button
              className={recording ? sessionStyles.stop : sessionStyles.primary}
              variant={recording ? 'soft' : 'solid'}
              disabled={processing || waiting}
              onClick={() =>
                void (recording
                  ? voice.stop()
                  : voice.hasPending && !rerecord
                    ? voice.retry()
                    : voice.start())
              }
            >
              {recording && (
                <Square size={14} fill="currentColor" aria-hidden="true" />
              )}
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
                        : micError
                          ? 'trainer.retry'
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
}: {
  task: DiagnosticTask
  transcript?: string
}) {
  const { t } = useTranslation()
  return (
    <TrainerQuestion
      badge={t(`diagnostic.${task.role}`)}
      question={task.question}
      options={task.options}
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
      <Text as="p" className={styles.summaryHelp}>
        {t('diagnostic.completedHelp')}
      </Text>
      <div className={styles.summaryScore}>
        <Text>
          {value.diagnostic_score} / {value.maximum_score}
        </Text>
        <Text>{t('diagnostic.totalScore')}</Text>
        <Button
          size="3"
          className={diagnosticStyles.toTraining}
          onClick={() => onTraining(sessionId)}
        >
          {t('diagnostic.toTraining')}
          <ArrowRight size={18} aria-hidden="true" />
        </Button>
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
                    <Heading as="h3">{answer.task.question}</Heading>
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
                  <Heading as="h2">
                    {t('diagnostic.skippedCompetencies')}
                  </Heading>
                  <ul>
                    {value.skipped_competencies.map((item) => (
                      <li key={item.competency_id}>{item.competency_name}</li>
                    ))}
                  </ul>
                </section>
              )}
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
