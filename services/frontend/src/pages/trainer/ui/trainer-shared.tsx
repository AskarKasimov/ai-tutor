import { Heading, Text } from '@radix-ui/themes'
import { CircleAlert, GraduationCap, LoaderCircle, Mic } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AccountMenu } from '@/features/auth'
import { VoiceIllustration } from './trainer-answer'
import styles from './trainer-layout.module.scss'

export function TrainerShell({
  title,
  subject,
  progress,
  children,
}: {
  title: string
  subject: string
  progress?: { label: string; current?: number; total?: number }
  children: ReactNode
}) {
  const { t } = useTranslation()
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
            {title}
          </Text>
          <Heading as="h2">{subject}</Heading>
        </div>
        {progress && (
          <div className={styles.progress}>
            <Text as="p" aria-live="polite">
              {progress.label}
            </Text>
            {progress.current !== undefined && progress.total !== undefined && (
              <div
                className={styles.progressTrack}
                role="progressbar"
                aria-label={t('session.progressLabel')}
                aria-valuemin={0}
                aria-valuemax={progress.total}
                aria-valuenow={progress.current}
              >
                <span
                  className={styles.progressFill}
                  style={{
                    width: `${progress.total ? (progress.current / progress.total) * 100 : 0}%`,
                  }}
                />
              </div>
            )}
          </div>
        )}
      </aside>
      <div className={styles.content}>
        <header className={styles.header}>
          <p className={styles.pageContext}>
            <span className={styles.pageContextLabel}>{title}</span>
            <span className={styles.pageContextDot} aria-hidden="true" />
            <span className={styles.pageContextTitle}>{subject}</span>
          </p>
          <AccountMenu />
        </header>
        {children}
      </div>
    </div>
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
  eyebrow: string
  eyebrowHeading?: boolean
  question: string
  options: string[]
  children?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <section className={styles.question}>
      <div className={styles.questionToolbar}>
        <Text className={styles.questionBadge}>{badge}</Text>
      </div>
      {eyebrowHeading ? (
        <Heading as="h2" className={styles.eyebrow}>
          {eyebrow}
        </Heading>
      ) : (
        <Text as="p" className={styles.eyebrow}>
          {eyebrow}
        </Text>
      )}
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
                  {index + 1}
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

export function TrainerAnswerPanel({
  error = false,
  children,
}: {
  error?: boolean
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <aside
      className={`${styles.answerPanel} ${error ? styles.answerError : ''}`}
      aria-label={t('trainer.answerArea')}
    >
      {children}
    </aside>
  )
}

export function TrainerScore({
  score,
  maxScore,
  verdict,
}: {
  score: number
  maxScore: number
  verdict: string
}) {
  return (
    <div
      className={`${styles.score} ${score === 0 ? styles.scoreIncorrect : score < maxScore ? styles.scorePartial : ''}`}
    >
      <Text>
        {score} / {maxScore}
      </Text>
      <Text className={styles.verdict}>{verdict}</Text>
    </div>
  )
}

export function TrainerCapturePanel({
  stage,
  seconds,
  stream,
  readyHelp,
  errorMessage,
  audioUrl,
  children,
}: {
  stage: 'ready' | 'permission' | 'recording' | 'processing' | 'error'
  seconds: string
  stream?: MediaStream
  readyHelp: string
  errorMessage?: string
  audioUrl?: string
  children: ReactNode
}) {
  const { t } = useTranslation()
  const recording = stage === 'recording'
  const processing = stage === 'processing'
  const waiting = stage === 'permission'
  const failed = stage === 'error'
  return (
    <TrainerAnswerPanel error={failed}>
      <Text as="p" className={styles.eyebrow}>
        {t('trainer.yourAnswer')}
      </Text>
      <VoiceIllustration stream={stream} />
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
      {errorMessage ? (
        <Text as="p" role="alert" className={styles.answerMessage}>
          <CircleAlert size={16} aria-hidden="true" />
          {errorMessage}
        </Text>
      ) : (
        <Text as="p" className={styles.instructions}>
          {t(
            recording
              ? 'trainer.recordingHelp'
              : processing
                ? 'trainer.processingHelp'
                : waiting
                  ? 'trainer.permissionHelp'
                  : readyHelp,
          )}
        </Text>
      )}
      {recording ? (
        <div className={styles.recordingTimer}>
          <span />
          {seconds}
        </div>
      ) : processing || waiting ? (
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
      {children}
      {audioUrl && failed && (
        <div className={styles.audio}>
          <audio
            controls
            src={audioUrl}
            aria-label={t('trainer.listenRecording')}
          />
        </div>
      )}
      {!failed && (
        <Text as="p" className={styles.microphoneStatus} aria-live="polite">
          {t(
            recording
              ? 'session.recordingStatus'
              : processing
                ? 'session.processingStatus'
                : waiting
                  ? 'trainer.allow'
                  : 'session.microphoneReady',
          )}
        </Text>
      )}
    </TrainerAnswerPanel>
  )
}
