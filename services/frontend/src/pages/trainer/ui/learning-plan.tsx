import { Button, Heading, Text } from '@radix-ui/themes'
import {
  ArrowRight,
  Check,
  CircleAlert,
  Clock,
  Flame,
  Lock,
  LoaderCircle,
  Mic,
  RotateCcw,
  Rocket,
  Square,
  Trophy,
  Zap,
} from 'lucide-react'
import {
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LearningPlanApiError,
  type LearningItem,
  type LearningMessage,
  type LearningPlan,
} from '@/entities/learning-plan'
import {
  useCreateLearningPlan,
  useLearningPlanQuery,
  useLessonActions,
  useLessonMessages,
  useVoiceRecorder,
} from '@/features/learning-plan'
import { LiveWaveform } from './trainer-answer'
import { TrainerProgress, TrainerShell } from './trainer-shared'
import { MarkdownContent } from '@/shared/ui'
import styles from './learning-plan.module.scss'

const hourOptions = [
  { hours: 1, icon: <Zap size={26} fill="currentColor" /> },
  { hours: 3, icon: <Flame size={26} /> },
  { hours: 5, icon: <Rocket size={26} /> },
  { hours: 10, icon: <Trophy size={26} /> },
] as const

export function LearningPlanEntry({
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
  const query = useLearningPlanQuery(userId, diagnosticId)
  const create = useCreateLearningPlan(userId, diagnosticId)
  if (query.data)
    return (
      <LearningPlanScreen
        userId={userId}
        plan={query.data}
        subjectName={subjectName}
        onBack={onBack}
      />
    )
  if (query.data === null)
    return (
      <HoursForm
        pending={create.isPending ? create.variables : undefined}
        failed={create.isError}
        onChoose={(hours) => create.mutate(hours)}
        onBack={onBack}
      />
    )
  return (
    <TrainerShell
      title={t('learning.title')}
      subject={subjectName}
      crumb={t('learning.title')}
      onBack={onBack}
    >
      <main className={styles.notice}>
        {query.isError ? (
          <>
            <Text role="alert">{t('learning.loadError')}</Text>
            <Button onClick={() => void query.refetch()}>
              {t('trainer.retry')}
            </Button>
          </>
        ) : (
          <Text role="status">{t('learning.loading')}</Text>
        )}
      </main>
    </TrainerShell>
  )
}

// A full-screen gradient question with large tiles: one click builds the plan.
function HoursForm({
  pending,
  failed,
  onChoose,
  onBack,
}: {
  pending?: number
  failed: boolean
  onChoose: (hours: number) => void
  onBack: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.hours}>
      <button type="button" className={styles.hoursBack} onClick={onBack}>
        {t('learning.back')}
      </button>
      <div className={styles.hoursContent}>
        <span className={styles.hoursEyebrow}>
          <Clock size={16} aria-hidden="true" />
          {t('learning.hoursEyebrow')}
        </span>
        <Heading as="h1" className={styles.hoursTitle}>
          {t('learning.hoursTitle')}
        </Heading>
        <Text as="p" className={styles.hoursText}>
          {t('learning.hoursText')}
        </Text>
        <div className={styles.tiles}>
          {hourOptions.map(({ hours, icon }) => (
            <button
              key={hours}
              type="button"
              className={styles.tile}
              data-hours={hours}
              disabled={pending !== undefined}
              aria-busy={pending === hours}
              onClick={() => onChoose(hours)}
            >
              <span className={styles.tileIcon} aria-hidden="true">
                {pending === hours ? (
                  <LoaderCircle size={26} className={styles.spinner} />
                ) : (
                  icon
                )}
              </span>
              <span className={styles.tileHours}>
                {hours === 10
                  ? t('learning.hoursMore', { count: hours })
                  : t('learning.hoursUnit', { count: hours })}
              </span>
              <span className={styles.tileLabel}>
                {t(`learning.hoursOption.${hours}`)}
              </span>
              <span className={styles.tileHint}>
                {t(`learning.hoursHint.${hours}`)}
              </span>
            </button>
          ))}
        </div>
        {pending !== undefined && (
          <Text as="p" role="status" className={styles.hoursStatus}>
            {t('learning.building')}
          </Text>
        )}
        {failed && (
          <Text as="p" role="alert" className={styles.hoursStatus}>
            {t('learning.createError')}
          </Text>
        )}
      </div>
    </div>
  )
}

function errorCode(error: unknown) {
  return error instanceof LearningPlanApiError ? error.code : ''
}

function LearningPlanScreen({
  userId,
  plan,
  subjectName,
  onBack,
}: {
  userId: string
  plan: LearningPlan
  subjectName: string
  onBack: () => void
}) {
  const { t } = useTranslation()
  // null follows the current topic; a passed topic can be opened to read it.
  const [viewed, setViewed] = useState<number | null>(null)
  const openFor = useRef<number | null>(null)
  const index = viewed ?? plan.current_index
  const item = index === null ? undefined : plan.items[index]
  const isCurrent = index !== null && index === plan.current_index
  const [celebration, setCelebration] = useState<CelebrationData | null>(null)
  // A correct answer does not jump to the next topic: the passed topic stays
  // on screen under a celebration until the student continues. The next
  // lesson is prepared meanwhile.
  const actions = useLessonActions(userId, plan, (before, after) => {
    const passed = before.current_index
    if (passed === null || after.items[passed]?.status !== 'passed') return
    setViewed(passed)
    setCelebration({
      index: passed,
      outcome: after.items[passed].outcome_name,
      attempts: after.items[passed].attempts,
      from: before.progress_percent,
      to: after.progress_percent,
      finished: after.status === 'completed',
    })
    if (after.current_index !== null) {
      openFor.current = after.current_index
      actions.open.mutate()
    }
  })
  const messages = useLessonMessages(
    userId,
    plan.id,
    index,
    isCurrent &&
      (actions.open.isPending ||
        actions.answer.isPending ||
        actions.retry.isPending),
  )
  const { mutate: open } = actions.open
  // The current lesson is written once, as soon as its topic is reached.
  useEffect(() => {
    if (
      !isCurrent ||
      index === null ||
      !messages.isSuccess ||
      messages.data.some((message) => message.kind === 'practice') ||
      openFor.current === index
    )
      return
    openFor.current = index
    open()
  }, [isCurrent, index, messages.isSuccess, messages.data, open])

  return (
    <TrainerShell
      title={t('learning.title')}
      subject={subjectName}
      crumb={t('learning.title')}
      onBack={onBack}
      progress={
        <TrainerProgress
          label={t('learning.progress')}
          percent={plan.progress_percent}
        />
      }
    >
      {celebration && (
        <Celebration
          data={celebration}
          onContinue={() => {
            setCelebration(null)
            setViewed(null)
          }}
        />
      )}
      <div className={styles.layout}>
        <Outline
          plan={plan}
          selected={index}
          onSelect={(next) =>
            setViewed(next === plan.current_index ? null : next)
          }
        />
        <section className={styles.chatPanel} aria-label={item?.outcome_name}>
          {plan.items.length === 0 ? (
            <Done plan={plan} />
          ) : !item ? (
            <Done plan={plan} />
          ) : (
            <>
              <Chat
                key={index}
                messages={messages.data ?? []}
                loading={messages.isPending}
                pendingAnswer={isCurrent ? plan.pending_answer : null}
                thinking={
                  isCurrent &&
                  (actions.open.isPending ||
                    actions.answer.isPending ||
                    actions.retry.isPending)
                }
                answerPending={
                  isCurrent &&
                  actions.answer.isPending &&
                  messages.data?.at(-1)?.role !== 'user'
                }
              />
              {isCurrent ? (
                <LessonControls
                  plan={plan}
                  actions={actions}
                  lessonReady={
                    !!messages.data?.some(
                      (message) => message.kind === 'practice',
                    )
                  }
                  onReopen={() => {
                    openFor.current = index
                    open()
                  }}
                />
              ) : (
                <Text as="p" className={styles.readOnly}>
                  {t(
                    item.status === 'pending'
                      ? 'learning.locked'
                      : 'learning.history',
                  )}
                </Text>
              )}
            </>
          )}
        </section>
      </div>
    </TrainerShell>
  )
}

function StatusIcon({ status }: { status: LearningItem['status'] }) {
  return (
    <span className={styles.statusIcon} data-status={status} aria-hidden="true">
      {status === 'passed' ? (
        <Check size={14} strokeWidth={3} />
      ) : status === 'review' ? (
        <RotateCcw size={13} strokeWidth={2.6} />
      ) : status === 'pending' ? (
        <Lock size={12} />
      ) : (
        <span className={styles.currentDot} />
      )}
    </span>
  )
}

// Competency → outcomes, top-down. Passed topics open read-only; future ones
// are closed.
function Outline({
  plan,
  selected,
  onSelect,
}: {
  plan: LearningPlan
  selected: number | null
  onSelect: (index: number) => void
}) {
  const { t } = useTranslation()
  const groups = useMemo(() => {
    const planned = plan.items.filter((item) => item.in_plan)
    const extra = plan.items.filter((item) => !item.in_plan)
    const byCompetency = (items: LearningItem[]) => {
      const result: { name: string; items: LearningItem[] }[] = []
      for (const item of items) {
        const last = result.at(-1)
        if (last?.name === item.competency_name) last.items.push(item)
        else result.push({ name: item.competency_name, items: [item] })
      }
      return result
    }
    return { planned: byCompetency(planned), extra: byCompetency(extra) }
  }, [plan.items])
  const renderGroup = (group: { name: string; items: LearningItem[] }) => (
    <li key={group.name + group.items[0].index} className={styles.group}>
      <Text className={styles.groupName}>{group.name}</Text>
      <ol className={styles.outcomes}>
        {group.items.map((item) => (
          <li key={item.index}>
            <button
              type="button"
              className={styles.outcome}
              data-status={item.status}
              data-extra={!item.in_plan || undefined}
              aria-current={selected === item.index ? 'step' : undefined}
              disabled={item.status === 'pending'}
              onClick={() => onSelect(item.index)}
            >
              <StatusIcon status={item.status} />
              <span className={styles.outcomeText}>
                <span className={styles.outcomeName}>{item.outcome_name}</span>
                <span className={styles.outcomeMeta}>
                  {t('learning.minutes', { count: item.theory_minutes })}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ol>
    </li>
  )
  return (
    <nav className={styles.outline} aria-label={t('learning.outline')}>
      <Heading as="h2" size="3" className={styles.outlineTitle}>
        {t('learning.outline')}
      </Heading>
      <ol className={styles.groups}>{groups.planned.map(renderGroup)}</ol>
      {groups.extra.length > 0 && (
        <>
          <Text className={styles.extraTitle}>{t('learning.extra')}</Text>
          <ol className={styles.groups}>{groups.extra.map(renderGroup)}</ol>
        </>
      )}
    </nav>
  )
}

function Paragraphs({ text }: { text: string }) {
  return (
    <>
      {text
        .split(/\n\s*\n/)
        .filter((part) => part.trim())
        .map((part, i) => (
          <p key={i} className={styles.paragraph}>
            {part.trim()}
          </p>
        ))}
    </>
  )
}

function Chat({
  messages,
  loading,
  pendingAnswer,
  thinking,
  answerPending,
}: {
  messages: LearningMessage[]
  loading: boolean
  pendingAnswer: string | null
  thinking: boolean
  answerPending: boolean
  writingLesson: boolean
}) {
  const { t } = useTranslation()
  const chat = useRef<HTMLOListElement>(null)
  const followOutput = useRef(true)
  const [historyEnd, setHistoryEnd] = useState<number | null>(() =>
    loading ? null : (messages.at(-1)?.sequence ?? 0),
  )
  useLayoutEffect(() => {
    if (!loading && historyEnd === null)
      setHistoryEnd(messages.at(-1)?.sequence ?? 0)
  }, [loading, messages, historyEnd])
  useLayoutEffect(() => {
    const element = chat.current
    if (element && followOutput.current)
      element.scrollTop = element.scrollHeight
  }, [loading, messages.length, pendingAnswer, thinking])
  const label = (message: LearningMessage) =>
    message.kind === 'theory'
      ? t('learning.theory')
      : message.kind === 'practice'
        ? t('learning.practice')
        : message.role === 'user'
          ? t('learning.you')
          : t('learning.theory')
  return (
    <ol
      ref={chat}
      className={styles.messages}
      aria-live="polite"
      onScroll={(event) => {
        const element = event.currentTarget
        followOutput.current =
          element.scrollHeight - element.scrollTop - element.clientHeight <= 40
      }}
    >
      {loading && (
        <li className={styles.assistant}>
          <Text role="status">{t('learning.loading')}</Text>
        </li>
      )}
      {messages.map((message) => {
        return (
          <li
            key={message.sequence}
            className={message.role === 'user' ? styles.user : styles.assistant}
            data-kind={message.kind}
            data-enter={historyEnd !== null && message.sequence > historyEnd}
          >
            <span className={styles.role}>{label(message)}</span>
            {message.role === 'assistant' ? (
              <MarkdownContent
                text={message.text}
                className={styles.lessonContent}
              />
            ) : (
              <Paragraphs text={message.text} />
            )}
          </li>
        )
      })}
      {pendingAnswer &&
        !messages.some(
          (m) => m.kind === 'answer' && m.text === pendingAnswer,
        ) && (
          <li className={styles.user} data-enter="true">
            <span className={styles.role}>{t('learning.you')}</span>
            <Paragraphs text={pendingAnswer} />
          </li>
        )}
      {thinking && (
        <li
          data-enter="true"
          className={[
            answerPending ? styles.user : styles.assistant,
            styles.thinking,
          ].join(' ')}
        >
          <span className={styles.role}>
            {t(answerPending ? 'learning.you' : 'learning.teacher')}
          </span>
          {writingLesson && (
            <Text size="2">
              {t(
                messages.some((message) => message.kind === 'theory')
                  ? 'learning.preparingPractice'
                  : 'learning.writingLesson',
              )}
            </Text>
          )}
          <span className={styles.dots} role="status">
            <span />
            <span />
            <span />
            <span className={styles.srOnly}>{t('learning.thinking')}</span>
          </span>
        </li>
      )}
    </ol>
  )
}

function seconds(value: number) {
  const minutes = Math.floor(value / 60)
  return `${minutes}:${String(value % 60).padStart(2, '0')}`
}

// The voice bar under the chat: the microphone, its hint or timer with the
// live waveform, and recovery actions for a failed lesson or check.
function LessonControls({
  plan,
  actions,
  lessonReady,
  onReopen,
}: {
  plan: LearningPlan
  actions: ReturnType<typeof useLessonActions>
  lessonReady: boolean
  onReopen: () => void
}) {
  const { t } = useTranslation()
  const { open, answer, retry } = actions
  const recorder = useVoiceRecorder(
    `${plan.id}:${plan.current_index}`,
    (audio) => answer.mutate(audio),
  )
  const busy =
    open.isPending || answer.isPending || retry.isPending || recorder.requesting
  const waiting = plan.pending_answer !== null
  const canRecord = lessonReady && !busy && !waiting
  let alert: ReactNode = null
  if (open.isError && !lessonReady)
    alert = (
      <>
        {t('learning.openError')}
        <Button size="1" variant="soft" onClick={onReopen}>
          {t('trainer.retry')}
        </Button>
      </>
    )
  else if (waiting && !busy)
    alert = (
      <>
        {t('learning.answerError')}
        <Button size="1" variant="soft" onClick={() => retry.mutate()}>
          {t('learning.retryCheck')}
        </Button>
      </>
    )
  else if (answer.isError && !busy)
    alert = t(
      errorCode(answer.error) === 'NO_SPEECH_DETECTED'
        ? 'learning.noSpeech'
        : 'learning.answerError',
    )
  else if (recorder.error)
    alert = t(
      recorder.error === 'denied'
        ? 'learning.micDenied'
        : 'learning.micUnavailable',
    )
  const state = recorder.recording ? 'recording' : busy ? 'busy' : 'ready'
  return (
    <div className={styles.composer}>
      {alert && (
        <div className={styles.alert} role="alert">
          <CircleAlert size={16} aria-hidden="true" />
          {alert}
        </div>
      )}
      <div className={styles.bar}>
        <button
          type="button"
          className={styles.barMic}
          data-state={state}
          disabled={!recorder.recording && !canRecord}
          aria-label={t(
            recorder.recording
              ? 'learning.stopRecording'
              : 'learning.recordAnswer',
          )}
          onClick={() =>
            recorder.recording ? recorder.stop() : void recorder.start()
          }
        >
          {busy ? (
            <LoaderCircle size={24} className={styles.spinner} />
          ) : recorder.recording ? (
            <Square size={20} fill="currentColor" />
          ) : (
            <Mic size={26} />
          )}
        </button>
        <div className={styles.barBody}>
          {recorder.recording ? (
            <>
              <Text className={styles.barTimer} role="timer">
                {seconds(recorder.elapsedSeconds)}
              </Text>
              <div className={styles.barWave} aria-hidden="true">
                <LiveWaveform stream={recorder.stream} height={36} />
              </div>
              <Text className={styles.barHint}>{t('learning.tapToStop')}</Text>
            </>
          ) : answer.isPending ? (
            <Text className={styles.barStatus} role="status">
              {t('learning.sending')}
            </Text>
          ) : busy ? (
            <Text className={styles.barStatus} role="status">
              {t('learning.thinking')}
            </Text>
          ) : (
            <Text className={styles.barHint}>{t('learning.tapToRecord')}</Text>
          )}
        </div>
      </div>
    </div>
  )
}

function Done({ plan }: { plan: LearningPlan }) {
  const { t } = useTranslation()
  const review = plan.items.filter((item) => item.status === 'review').length
  return (
    <div className={styles.done}>
      <span className={styles.doneIcon} aria-hidden="true">
        <Trophy size={34} />
      </span>
      <Heading as="h2" size="6">
        {plan.items.length ? t('learning.doneTitle') : t('learning.title')}
      </Heading>
      <Text as="p">
        {plan.items.length ? t('learning.doneText') : t('learning.empty')}
      </Text>
      {review > 0 && (
        <Text as="p" className={styles.doneReview}>
          {t('learning.doneReview', { count: review })}
        </Text>
      )}
    </div>
  )
}

type CelebrationData = {
  index: number
  outcome: string
  attempts: number
  from: number
  to: number
  finished: boolean
}

const confettiColors = [
  '#6366f1',
  '#8b5cf6',
  '#ec4899',
  '#f59e0b',
  '#10b981',
  '#3b82f6',
]
// Fixed pseudo-random pieces: the same burst on every render.
const confetti = Array.from({ length: 48 }, (_, i) => ({
  left: (i * 37) % 100,
  delay: ((i * 53) % 60) / 100,
  duration: 2.2 + ((i * 29) % 15) / 10,
  size: 6 + ((i * 17) % 7),
  turn: (i * 71) % 360,
  drift: ((i * 43) % 120) - 60,
  color: confettiColors[i % confettiColors.length],
  round: i % 3 === 0,
}))

// The plan progress counts up from the old to the new value.
function useCountUp(from: number, to: number, duration = 900) {
  const [value, setValue] = useState(from)
  useEffect(() => {
    let frame = 0
    const start = performance.now()
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / duration)
      const eased = 1 - (1 - t) ** 3
      setValue(Math.round(from + (to - from) * eased))
      if (t < 1) frame = requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
    return () => cancelAnimationFrame(frame)
  }, [from, to, duration])
  return value
}

// A modal reward for a passed topic: confetti, a trophy, praise and the plan
// progress growing; the student continues explicitly.
function Celebration({
  data,
  onContinue,
}: {
  data: CelebrationData
  onContinue: () => void
}) {
  const { t } = useTranslation()
  const progress = useCountUp(data.from, data.to)
  const titleId = useId()
  useEffect(() => {
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onContinue()
    }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [onContinue])
  return (
    <div className={styles.celebration}>
      <div className={styles.confetti} aria-hidden="true">
        {confetti.map((piece, i) => (
          <span
            key={i}
            style={
              {
                left: `${piece.left}%`,
                width: piece.size,
                height: piece.round ? piece.size : piece.size * 1.6,
                borderRadius: piece.round ? '50%' : 2,
                background: piece.color,
                animationDelay: `${piece.delay}s`,
                animationDuration: `${piece.duration}s`,
                '--turn': `${piece.turn}deg`,
                '--drift': `${piece.drift}px`,
              } as CSSProperties
            }
          />
        ))}
      </div>
      <div
        className={styles.celebrationCard}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <span className={styles.trophy} aria-hidden="true">
          <Trophy size={44} />
        </span>
        <Heading as="h2" id={titleId} className={styles.celebrationTitle}>
          {t(`learning.cheer.${data.index % 4}`)}
        </Heading>
        <Text as="p" className={styles.celebrationText}>
          {t('learning.topicPassed', { name: data.outcome })}
        </Text>
        <span className={styles.celebrationBadge}>
          <Flame size={16} aria-hidden="true" />
          {data.attempts <= 1
            ? t('learning.firstTry')
            : t('learning.persistence', { count: data.attempts })}
        </span>
        <div className={styles.celebrationProgress}>
          <div className={styles.celebrationProgressText}>
            <span>{t('learning.progress')}</span>
            <strong>{t('session.percent', { value: progress })}</strong>
          </div>
          <div
            className={styles.celebrationTrack}
            role="progressbar"
            aria-label={t('learning.progress')}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={data.to}
          >
            <div
              className={styles.celebrationFill}
              style={{ width: `${progress}%` }}
            />
          </div>
        </div>
        <button
          type="button"
          className={styles.celebrationButton}
          autoFocus
          onClick={onContinue}
        >
          {t(data.finished ? 'learning.toResult' : 'learning.continue')}
          <ArrowRight size={20} aria-hidden="true" />
        </button>
      </div>
    </div>
  )
}
