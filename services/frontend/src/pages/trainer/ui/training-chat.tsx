import { Button, Heading, Text } from '@radix-ui/themes'
import { LoaderCircle, Mic, Square } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrainingApiError } from '@/entities/training'
import type { TrainingChatMessage } from '@/entities/training'
import { useTrainingChat, useTrainingChatVoice } from '@/features/training'
import styles from './training-chat.module.scss'
import { LiveWaveform } from './trainer-answer'
import sessionStyles from './session.module.scss'

function flatten(pages: TrainingChatMessage[][] | undefined) {
  const unique = new Map<string, TrainingChatMessage>()
  for (const page of pages ?? [])
    for (const message of page) unique.set(message.id, message)
  return [...unique.values()].sort((a, b) => a.sequence - b.sequence)
}

export function TrainingChat({
  userId,
  sessionId,
  exerciseId,
  readOnly = false,
  sectionId = 'training-chat-title',
  onRecordingStart,
  onRecordingChange,
  onSkip,
  skipDisabled,
  skipError,
}: {
  userId: string
  sessionId: string
  exerciseId: string
  readOnly?: boolean
  sectionId?: string
  onRecordingStart?: () => void
  onRecordingChange?: (recording: boolean) => void
  onSkip?: () => void
  skipDisabled?: boolean
  skipError?: string
}) {
  const { t, i18n } = useTranslation()
  const chat = useTrainingChat(userId, sessionId, exerciseId)
  const [voiceProcessing, setVoiceProcessing] = useState(false)
  const messagesEnd = useRef<HTMLDivElement | null>(null)
  const messagesViewport = useRef<HTMLOListElement | null>(null)
  const followLatest = useRef(true)
  const voiceCapture = useRef<{
    revision: number
  } | null>(null)
  const messages = useMemo(
    () => flatten(chat.query.data?.pages.map((page) => page.items)),
    [chat.query.data],
  )
  const page = chat.query.data?.pages[0]
  const voice = useTrainingChatVoice(`${sessionId}:${exerciseId}`, (audio) => {
    const capture = voiceCapture.current
    voiceCapture.current = null
    if (!capture || readOnly) return
    setVoiceProcessing(true)
    void chat.send
      .mutateAsync({
        kind: 'message',
        source: 'voice',
        revision: capture.revision,
        audio,
      })
      .catch(() => {})
      .finally(() => setVoiceProcessing(false))
  })
  const recording = voice.recording
  const busy =
    chat.send.isPending ||
    chat.retry.isPending ||
    chat.cancel.isPending ||
    chat.next.isPending ||
    voiceProcessing ||
    recording ||
    voice.requesting

  useEffect(() => {
    setVoiceProcessing(false)
    voiceCapture.current = null
  }, [sessionId, exerciseId])

  useEffect(() => {
    onRecordingChange?.(recording || voice.requesting)
    return () => onRecordingChange?.(false)
  }, [onRecordingChange, recording, voice.requesting])

  useEffect(() => {
    if (followLatest.current) {
      messagesEnd.current?.scrollIntoView?.({
        behavior: 'smooth',
        block: 'end',
      })
    }
  }, [messages.length])

  const startVoice = () => {
    if (!page || readOnly || busy) return
    onRecordingStart?.()
    voiceCapture.current = { revision: page.revision }
    void voice.start()
  }

  const loadOlder = async () => {
    const viewport = messagesViewport.current
    if (!viewport) {
      await chat.query.fetchNextPage()
      return
    }
    followLatest.current = false
    const previousHeight = viewport.scrollHeight
    const previousTop = viewport.scrollTop
    await chat.query.fetchNextPage()
    requestAnimationFrame(() => {
      viewport.scrollTop =
        previousTop + (viewport.scrollHeight - previousHeight)
    })
  }

  const pending = page?.pending_turn
  const error =
    chat.send.error ?? chat.retry.error ?? chat.cancel.error ?? chat.next.error
  const errorKey =
    error instanceof TrainingApiError && error.status === 503
      ? 'training.chatUnavailable'
      : error instanceof TrainingApiError && error.status === 504
        ? 'training.chatTimeout'
        : error instanceof TrainingApiError && error.status === 409
          ? 'training.chatConflict'
          : 'training.chatError'
  const recordingTime = `${Math.floor(voice.elapsedSeconds / 60)
    .toString()
    .padStart(
      2,
      '0',
    )}:${(voice.elapsedSeconds % 60).toString().padStart(2, '0')}`

  return (
    <section className={styles.chat} aria-labelledby={sectionId}>
      <div className={styles.heading}>
        <Heading id={sectionId} as="h2">
          {t('training.chatTitle')}
        </Heading>
        <Text as="p" className={sessionStyles.panelHelp}>
          {t(readOnly ? 'training.chatHistoryHelp' : 'training.chatHelp')}
        </Text>
      </div>
      {chat.query.isPending ? (
        <Text role="status">{t('training.chatLoading')}</Text>
      ) : null}
      {chat.query.isError ? (
        <div className={styles.notice} role="alert">
          {t('training.chatError')}{' '}
          <Button variant="soft" onClick={() => void chat.query.refetch()}>
            {t('trainer.retry')}
          </Button>
        </div>
      ) : null}
      {chat.query.hasNextPage ? (
        <Button
          variant="ghost"
          onClick={() => void loadOlder()}
          disabled={chat.query.isFetchingNextPage}
        >
          {t('training.chatOlder')}
        </Button>
      ) : null}
      <ol
        ref={messagesViewport}
        className={styles.messages}
        onScroll={(event) => {
          const node = event.currentTarget
          followLatest.current =
            node.scrollHeight - node.scrollTop - node.clientHeight < 72
        }}
      >
        {messages.map((message) => {
          const date = new Date(message.created_at * 1000)
          return (
            <li
              key={message.id}
              className={
                message.role === 'user' ? styles.user : styles.assistant
              }
            >
              <Text className={styles.role}>
                {t(
                  message.role === 'user'
                    ? 'training.chatYou'
                    : 'training.chatTutor',
                )}
              </Text>
              <Text>{message.text}</Text>
              <time className={styles.timestamp} dateTime={date.toISOString()}>
                {new Intl.DateTimeFormat(i18n.language, {
                  hour: '2-digit',
                  minute: '2-digit',
                }).format(date)}
              </time>
            </li>
          )
        })}
        <div ref={messagesEnd} />
      </ol>
      <div className={styles.announcement} role="status" aria-live="polite">
        {recording
          ? t('training.chatRecordingStatus', { time: recordingTime })
          : voice.requesting
            ? t('training.chatMicPermission')
            : voiceProcessing
              ? t('training.chatProcessing')
              : ''}
      </div>
      {!readOnly &&
      page?.status === 'active' &&
      page.latest_check?.revision === page.revision &&
      !pending ? (
        <Button
          className={styles.next}
          disabled={busy}
          onClick={() => void chat.next.mutateAsync(page.revision)}
        >
          {t('training.next')}
        </Button>
      ) : null}
      {!readOnly && pending ? (
        <div className={styles.pending} role="status">
          <Text>
            {pending.status === 'processing'
              ? t('training.chatProcessing')
              : t('training.chatFailed')}
          </Text>
          {pending.status === 'failed' ? (
            <Button
              variant="soft"
              onClick={() => void chat.retry.mutateAsync(pending.id)}
            >
              {t('training.retrySubmit')}
            </Button>
          ) : null}
          <Button
            variant="ghost"
            color="gray"
            onClick={() => void chat.cancel.mutateAsync(pending.id)}
          >
            {t('training.chatCancel')}
          </Button>
        </div>
      ) : null}
      {error ? (
        <Text role="alert" className={styles.error}>
          {t(errorKey)}
        </Text>
      ) : null}
      {readOnly || page?.status === 'closed' ? (
        <Text>{t('training.chatClosed')}</Text>
      ) : (
        <div className={styles.composer}>
          <div className={styles.capture}>
            <button
              type="button"
              className={`${sessionStyles.mic} ${styles.microphone}`}
              data-state={
                recording ? 'recording' : busy || pending ? 'busy' : 'ready'
              }
              disabled={!recording && (!page || busy || !!pending)}
              onClick={recording ? () => voice.stop() : startVoice}
              aria-pressed={recording}
              aria-label={t(
                recording
                  ? 'training.chatStopRecording'
                  : 'training.chatRecord',
              )}
            >
              {recording ? (
                <Square size={20} fill="currentColor" aria-hidden="true" />
              ) : busy || pending ? (
                <LoaderCircle
                  className={sessionStyles.spinner}
                  size={26}
                  aria-hidden="true"
                />
              ) : (
                <Mic size={28} aria-hidden="true" />
              )}
            </button>
            <div
              className={styles.waveform}
              role="img"
              aria-label={t('training.waveform')}
            >
              <LiveWaveform stream={voice.stream} height={64} />
            </div>
            {recording ? (
              <Text className={styles.timer} role="timer">
                {recordingTime}
              </Text>
            ) : voice.requesting || voiceProcessing || chat.send.isPending ? (
              <Text className={styles.captureStatus} role="status">
                {t(
                  voice.requesting
                    ? 'training.chatMicPermission'
                    : 'training.chatProcessing',
                )}
              </Text>
            ) : null}
            {voice.error ? (
              <Text
                role="alert"
                className={`${styles.error} ${styles.captureStatus}`}
              >
                {t(
                  voice.error === 'denied'
                    ? 'training.chatMicDenied'
                    : 'trainer.unavailable',
                )}
              </Text>
            ) : null}
          </div>
        </div>
      )}
      {!readOnly && onSkip ? (
        <Button
          variant="outline"
          color="gray"
          className={sessionStyles.skip}
          disabled={skipDisabled || busy || !!pending}
          onClick={onSkip}
        >
          {t('training.skipExercise')}
        </Button>
      ) : null}
      {skipError ? (
        <Text role="alert" className={styles.error}>
          {skipError}
        </Text>
      ) : null}
    </section>
  )
}
