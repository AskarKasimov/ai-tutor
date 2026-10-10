import { Button, Heading, Text } from '@radix-ui/themes'
import {
  ChevronDown,
  ChevronRight,
  ChevronUp,
  CircleAlert,
  GraduationCap,
  LoaderCircle,
  MicOff,
  Play,
  Square,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AccountMenu } from '@/features/auth'
import { LiveWaveform } from './trainer-answer'
import styles from './session.module.scss'

export const sessionStyles = styles

export type NavigatorState =
  | 'correct'
  | 'partial'
  | 'incorrect'
  | 'skipped'
  | 'current'
  | 'locked'
  | 'done'

export type NavigatorItem = { number: number; state: NavigatorState }

export function TrainerShell({
  title,
  subject,
  crumb,
  onBack,
  navigator,
  children,
}: {
  title: string
  subject: string
  crumb?: string
  onBack?: () => void
  navigator?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div data-trainer-app className={styles.shell}>
      <div className={styles.frame}>
        <header className={styles.header}>
          {onBack ? (
            // The product logo returns to the student home.
            <button
              type="button"
              className={`${styles.brand} ${styles.brandLink}`}
              aria-label={t('home.back')}
              title={t('home.back')}
              onClick={onBack}
            >
              <span className={styles.logo} aria-hidden="true">
                <GraduationCap size={18} />
              </span>
              <Text>{t('trainer.title')}</Text>
            </button>
          ) : (
            <div className={styles.brand}>
              <span className={styles.logo} aria-hidden="true">
                <GraduationCap size={18} />
              </span>
              <Text>{t('trainer.title')}</Text>
            </div>
          )}
          {/* The course leads home; the current step is a non-clickable pill. */}
          <nav className={styles.crumbs} aria-label={title}>
            {onBack ? (
              <button
                type="button"
                className={styles.crumbLink}
                onClick={onBack}
              >
                {subject}
              </button>
            ) : (
              <span className={styles.crumbText}>{subject}</span>
            )}
            {crumb && (
              <>
                <ChevronRight
                  size={16}
                  className={styles.crumbDivider}
                  aria-hidden="true"
                />
                <strong className={styles.crumbCurrent} aria-current="page">
                  {crumb}
                </strong>
              </>
            )}
          </nav>
          <div className={styles.account}>
            <AccountMenu />
          </div>
        </header>
        <div className={`${styles.body} ${navigator ? '' : styles.bodyWide}`}>
          {navigator}
          <div className={styles.stage}>{children}</div>
        </div>
      </div>
    </div>
  )
}

export function TrainerWorkspace({ children }: { children: ReactNode }) {
  return <main className={styles.workspace}>{children}</main>
}

// Numbers of the session questions; arrows scroll the list, it is not
// interactive because the diagnostic order is fixed.
export function QuestionNavigator({
  label,
  items,
  progress,
}: {
  label: string
  items: NavigatorItem[]
  progress?: { current: number; total: number }
}) {
  const { t } = useTranslation()
  const list = useRef<HTMLOListElement>(null)
  const [edges, setEdges] = useState({ start: true, end: true })
  const measure = useCallback(() => {
    const node = list.current
    if (!node) return
    const vertical = node.scrollHeight > node.clientHeight + 1
    const scrolled = vertical ? node.scrollTop : node.scrollLeft
    const extent = vertical
      ? node.scrollHeight - node.clientHeight
      : node.scrollWidth - node.clientWidth
    setEdges({ start: scrolled <= 1, end: scrolled >= extent - 1 })
  }, [])
  const current = items.find((item) => item.state === 'current')?.number
  useEffect(() => {
    const node = list.current
    node
      ?.querySelector('[aria-current="step"]')
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
    measure()
  }, [current, items.length, measure])
  useEffect(() => {
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [measure])
  const scroll = (direction: 1 | -1) => {
    const node = list.current
    if (!node) return
    const vertical = node.scrollHeight > node.clientHeight + 1
    node.scrollBy?.(
      vertical ? { top: direction * 120 } : { left: direction * 120 },
    )
  }
  return (
    <nav className={styles.navigator} aria-label={label}>
      <button
        type="button"
        className={styles.navArrow}
        aria-label={t('session.scrollBack')}
        disabled={edges.start}
        onClick={() => scroll(-1)}
      >
        <ChevronUp size={20} aria-hidden="true" />
      </button>
      <ol ref={list} className={styles.navList} onScroll={measure}>
        {items.map((item) => (
          <li
            key={item.number}
            className={styles.navItem}
            data-state={item.state}
            aria-current={item.state === 'current' ? 'step' : undefined}
            aria-label={t(`session.navigator.${item.state}`, {
              number: item.number,
            })}
          >
            {item.number}
          </li>
        ))}
      </ol>
      <button
        type="button"
        className={styles.navArrow}
        aria-label={t('session.scrollForward')}
        disabled={edges.end}
        onClick={() => scroll(1)}
      >
        <ChevronDown size={20} aria-hidden="true" />
      </button>
      {progress && (
        <div
          className={styles.visuallyHidden}
          role="progressbar"
          aria-label={t('session.progressLabel')}
          aria-valuemin={0}
          aria-valuemax={progress.total}
          aria-valuenow={progress.current}
        />
      )}
    </nav>
  )
}

export function TrainerQuestion({
  badge,
  eyebrow,
  eyebrowHeading = false,
  question,
  options,
  children,
}: {
  badge: string
  eyebrow?: string
  eyebrowHeading?: boolean
  question: string
  options: string[]
  children?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <section className={styles.question}>
      <div className={styles.eyebrowRow}>
        <Text className={styles.badge}>{badge}</Text>
        {eyebrow &&
          (eyebrowHeading ? (
            <Heading as="h2" className={styles.eyebrow}>
              {eyebrow}
            </Heading>
          ) : (
            <Text as="p" className={styles.eyebrow}>
              {eyebrow}
            </Text>
          ))}
      </div>
      <Heading
        as="h1"
        className={`${styles.questionTitle} ${question.length > 300 ? styles.longQuestion : ''}`}
      >
        {question}
      </Heading>
      {options.length > 0 && (
        <section
          aria-labelledby="answer-options-title"
          className={styles.options}
        >
          <Heading as="h2" id="answer-options-title" className={styles.eyebrow}>
            {t('trainer.optionsTitle')}
          </Heading>
          <ol className={styles.optionList}>
            {options.map((option, index) => (
              <li key={index} className={styles.option}>
                <span className={styles.optionLetter} aria-hidden="true">
                  {String.fromCharCode(65 + index)}
                </span>
                <Text>{option}</Text>
              </li>
            ))}
          </ol>
        </section>
      )}
      {children}
    </section>
  )
}

export function TrainerTranscript({ id, text }: { id: string; text?: string }) {
  const { t } = useTranslation()
  return (
    <section className={styles.transcript} aria-labelledby={id}>
      <Heading as="h2" id={id} className={styles.eyebrow}>
        {t('trainer.yourAnswer')}
      </Heading>
      <Text as="p" className={styles.transcriptText}>
        {text ?? t('diagnostic.transcriptPending')}
      </Text>
    </section>
  )
}

export function TrainerAnswerPanel({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <aside className={styles.panel} aria-label={t('trainer.answerArea')}>
      {children}
    </aside>
  )
}

export function TrainerScore({
  score,
  maxScore,
  verdict,
  skipped = false,
}: {
  score: number
  maxScore: number
  verdict: string
  skipped?: boolean
}) {
  const tone = skipped
    ? 'skipped'
    : score >= maxScore
      ? 'correct'
      : score === 0
        ? 'incorrect'
        : 'partial'
  return (
    <div className={styles.score} data-tone={tone}>
      <Text>
        {score} / {maxScore}
      </Text>
      <Text className={styles.verdict}>{verdict}</Text>
    </div>
  )
}

export function ListenButton({
  state,
  disabled,
  onClick,
}: {
  state: 'idle' | 'loading' | 'playing'
  disabled?: boolean
  onClick: () => void
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      className={styles.listen}
      disabled={disabled}
      onClick={onClick}
    >
      <span className={styles.listenIcon} aria-hidden="true">
        {state === 'loading' ? (
          <LoaderCircle size={16} className={styles.spinner} />
        ) : state === 'playing' ? (
          <Square size={12} fill="currentColor" />
        ) : (
          <Play size={14} fill="currentColor" />
        )}
      </span>
      {t(
        state === 'loading'
          ? 'trainer.cancelSpeech'
          : state === 'playing'
            ? 'trainer.stopSpeech'
            : 'trainer.playInstruction',
      )}
    </button>
  )
}

// The voice panel: instruction playback, live waveform with a timer and the
// answer actions; a failed microphone shows the crossed-out microphone state.
export function TrainerCapturePanel({
  stage,
  seconds,
  stream,
  speech,
  errorMessage,
  micError,
  audioUrl,
  onSkip,
  skipDisabled,
  children,
}: {
  stage: 'ready' | 'permission' | 'recording' | 'processing' | 'error'
  seconds: string
  stream?: MediaStream
  speech: ReactNode
  errorMessage?: string
  micError?: { title: string; help: string }
  audioUrl?: string
  onSkip?: () => void
  skipDisabled?: boolean
  children: ReactNode
}) {
  const { t } = useTranslation()
  const busy = stage === 'processing' || stage === 'permission'
  return (
    <TrainerAnswerPanel>
      <Heading as="h2" className={styles.panelTitle}>
        {t('session.answerByVoice')}
      </Heading>
      <Text as="p" className={styles.panelHelp}>
        {t('diagnostic.answerHelp')}
      </Text>
      {speech}
      {micError ? (
        <div className={styles.micError} role="alert">
          <span className={styles.micErrorIcon} aria-hidden="true">
            <MicOff size={44} />
          </span>
          <Heading as="h3">{micError.title}</Heading>
          <Text as="p">{micError.help}</Text>
        </div>
      ) : (
        <div className={styles.wave} aria-hidden={busy ? undefined : true}>
          {busy ? (
            <LoaderCircle size={40} className={styles.spinner} />
          ) : (
            <LiveWaveform stream={stream} />
          )}
          {busy ? (
            <Text className={styles.waveStatus} role="status">
              {t(
                stage === 'permission'
                  ? 'trainer.allow'
                  : 'session.processingStatus',
              )}
            </Text>
          ) : stage === 'recording' ? (
            <Text className={styles.timer}>{seconds}</Text>
          ) : null}
        </div>
      )}
      {errorMessage && !micError && (
        <Text as="p" role="alert" className={styles.alert}>
          <CircleAlert size={16} aria-hidden="true" />
          {errorMessage}
        </Text>
      )}
      <div className={styles.actions}>{children}</div>
      {audioUrl && stage === 'error' && (
        <div className={styles.audio}>
          <audio
            controls
            src={audioUrl}
            aria-label={t('trainer.listenRecording')}
          />
        </div>
      )}
      <div className={styles.spacer} />
      {onSkip && (
        <Button
          variant="outline"
          color="gray"
          className={styles.skip}
          disabled={skipDisabled}
          onClick={onSkip}
        >
          {t('session.skipQuestion')}
        </Button>
      )}
    </TrainerAnswerPanel>
  )
}
