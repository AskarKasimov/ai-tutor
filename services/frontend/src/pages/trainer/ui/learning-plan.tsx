import { Button, Heading, Text } from '@radix-ui/themes'
import {
  Check,
  CircleAlert,
  Clock,
  Flame,
  Lock,
  LoaderCircle,
  Mic,
  Play,
  RotateCcw,
  Rocket,
  Square,
  Trophy,
  Zap,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
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
  const index = viewed ?? plan.current_index
  const item = index === null ? undefined : plan.items[index]
  const isCurrent = index !== null && index === plan.current_index
  const actions = useLessonActions(userId, plan)
  const messages = useLessonMessages(
    userId,
    plan.id,
    index,
    isCurrent && actions.answer.isPending,
  )
  const { mutate: open } = actions.open
  const openFor = useRef<number | null>(null)
  // The current lesson is written once, as soon as its topic is reached.
  useEffect(() => {
    if (
      !isCurrent ||
      index === null ||
      !messages.isSuccess ||
      messages.data.length > 0 ||
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
              <header className={styles.chatHeader}>
                <Text className={styles.chatCompetency}>
                  {item.competency_name}
                </Text>
                <Heading as="h2" size="5">
                  {item.outcome_name}
                </Heading>
              </header>
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
                writingLesson={isCurrent && actions.open.isPending}
              />
              {isCurrent ? (
                <LessonControls
                  plan={plan}
                  actions={actions}
                  lessonReady={!!messages.data?.length}
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
        <Play size={11} fill="currentColor" />
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
                  {t('learning.minutes', {
                    theory: item.theory_minutes,
                    practice: item.practice_minutes,
                  })}
                  {' · '}
                  {t(`learning.status.${item.status}`)}
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
  writingLesson,
}: {
  messages: LearningMessage[]
  loading: boolean
  pendingAnswer: string | null
  thinking: boolean
  writingLesson: boolean
}) {
  const { t } = useTranslation()
  const end = useRef<HTMLLIElement>(null)
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: 'end', behavior: 'smooth' })
  }, [messages.length, thinking])
  const label = (message: LearningMessage) =>
    message.kind === 'theory'
      ? t('learning.theory')
      : message.kind === 'practice'
        ? t('learning.practice')
        : message.role === 'user'
          ? t('learning.you')
          : t('learning.teacher')
  return (
    <ol className={styles.messages} aria-live="polite">
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
          >
            <span className={styles.role}>{label(message)}</span>
            <Paragraphs text={message.text} />
          </li>
        )
      })}
      {pendingAnswer &&
        !messages.some(
          (m) => m.kind === 'answer' && m.text === pendingAnswer,
        ) && (
          <li className={styles.user}>
            <span className={styles.role}>{t('learning.you')}</span>
            <Paragraphs text={pendingAnswer} />
          </li>
        )}
      {thinking && (
        <li className={`${styles.assistant} ${styles.thinking}`}>
          <span className={styles.role}>{t('learning.teacher')}</span>
          {writingLesson && <Text size="2">{t('learning.writingLesson')}</Text>}
          <span className={styles.dots} role="status">
            <span />
            <span />
            <span />
            <span className={styles.srOnly}>{t('learning.thinking')}</span>
          </span>
        </li>
      )}
      <li ref={end} className={styles.end} aria-hidden="true" />
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
