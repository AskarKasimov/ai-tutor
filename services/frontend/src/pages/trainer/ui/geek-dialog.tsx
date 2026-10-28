import { Button, Heading, Text, TextArea } from '@radix-ui/themes'
import {
  ArrowRight,
  CircleAlert,
  LoaderCircle,
  Mic,
  Send,
  Square,
  Volume2,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import {
  DiagnosticApiError,
  type DiagnosticAnswer,
  type DiagnosticSession,
  type Turn,
} from '@/entities/diagnostic-session'
import {
  useAnswerMutation,
  useDiagnosticQuery,
  useTurnAudio,
  useVoiceAnswer,
  type TurnAudioState,
} from '@/features/diagnostic-session'
import { MarkdownContent, Toast } from '@/shared/ui'
import { LiveWaveform } from './trainer-answer'
import { TrainerProgress, TrainerShell } from './trainer-shared'
import styles from './geek.module.scss'

type Props = {
  userId: string
  sessionId: string
  subjectName: string
  onBack: () => void
  onResult: () => void
}

// The oral exam: Geek's turns on the left with their voice, the student's
// answers on the right and the answer field at the bottom.
export function GeekDialog({
  userId,
  sessionId,
  subjectName,
  onBack,
  onResult,
}: Props) {
  const { t } = useTranslation()
  const query = useDiagnosticQuery(userId, sessionId)
  const session = query.data
  return (
    <TrainerShell
      title={t('geek.title')}
      subject={subjectName}
      crumb={t('geek.crumb')}
      onBack={onBack}
      progress={
        session && (
          <TrainerProgress
            label={t('geek.progress')}
            percent={session.progressPercent}
          />
        )
      }
    >
      {session ? (
        // A failed background refresh keeps the dialogue, its draft and a
        // running recording; the error is shown above the answer field.
        <Dialogue
          userId={userId}
          session={session}
          refreshFailed={query.isError}
          onRefresh={() => void query.refetch()}
          onBack={onBack}
          onResult={onResult}
        />
      ) : query.isError ? (
        <main>
          <Heading as="h1" size="6">
            {t('geek.title')}
          </Heading>
          <Text as="p" role="alert">
            {t('geek.loadError')}
          </Text>
          <Button onClick={() => void query.refetch()}>
            {t('trainer.retry')}
          </Button>
        </main>
      ) : (
        <main>
          <Text role="status">{t('geek.loading')}</Text>
        </main>
      )}
    </TrainerShell>
  )
}

function Dialogue({
  userId,
  session,
  refreshFailed,
  onRefresh,
  onBack,
  onResult,
}: {
  userId: string
  session: DiagnosticSession
  refreshFailed: boolean
  onRefresh: () => void
  onBack: () => void
  onResult: () => void
}) {
  const { t } = useTranslation()
  const answer = useAnswerMutation(userId, session.id)
  const audio = useTurnAudio(session.id)
  // While the student records or checks a spoken answer, Geek stays silent.
  const [capturing, setCapturing] = useState(false)
  const dialog = useRef<HTMLElement>(null)
  const end = useRef<HTMLDivElement>(null)
  const followEnd = useRef(true)
  const scrollInitialized = useRef(false)
  const autoScrolling = useRef(false)
  // A rebuilt course graph cancels the diagnostic; the student learns it from
  // a pop-up the moment the dialogue sees it, not from a failed answer.
  const [courseUpdated, setCourseUpdated] = useState(false)
  const cancelled = session.status === 'cancelled'
  useEffect(() => {
    if (cancelled) setCourseUpdated(true)
  }, [cancelled])
  const closeNotice = useCallback(() => setCourseUpdated(false), [])
  const currentTurnId = session.currentTurnId
  const lastTurn = session.turns[session.turns.length - 1]
  // Text is available as soon as the answer arrives. Speech loads independently.
  const [transcribing, setTranscribing] = useState(false)

  // A new Geek turn is voiced at once; the first one after loading too, since
  // the student has just pressed start or continue.
  // Opening the transcript of a completed exam must not replay its last turn.
  const voiced = useRef(
    session.status === 'completed' ? (lastTurn?.id ?? '') : '',
  )
  const play = audio.play
  useEffect(() => {
    if (
      lastTurn?.speaker === 'geek' &&
      voiced.current !== lastTurn.id &&
      !capturing
    ) {
      voiced.current = lastTurn.id
      void play(lastTurn)
    }
  }, [lastTurn, play, capturing])
  const resultReady = session.status === 'completed'
  useEffect(() => {
    // On mobile the card grows with its contents and the document scrolls.
    const trackPageScroll = () => {
      const container = dialog.current
      if (
        autoScrolling.current ||
        !container ||
        container.scrollHeight > container.clientHeight
      )
        return
      const page = document.scrollingElement ?? document.documentElement
      followEnd.current =
        page.scrollHeight - page.scrollTop - page.clientHeight < 96
    }
    // Animation scroll events are not a request to stop following the chat.
    // The next user interaction hands control back to their scroll position.
    const resumeUserScroll = () => {
      autoScrolling.current = false
    }
    const inputEvents = [
      'wheel',
      'touchstart',
      'pointerdown',
      'keydown',
    ] as const
    for (const event of inputEvents)
      window.addEventListener(event, resumeUserScroll, { passive: true })
    window.addEventListener('scroll', trackPageScroll, { passive: true })
    return () => {
      window.removeEventListener('scroll', trackPageScroll)
      for (const event of inputEvents)
        window.removeEventListener(event, resumeUserScroll)
    }
  }, [])
  useLayoutEffect(() => {
    if (!followEnd.current) return
    const reducedMotion = window.matchMedia(
      '(prefers-reduced-motion: reduce)',
    ).matches
    const behavior =
      scrollInitialized.current && !reducedMotion ? 'smooth' : 'auto'
    autoScrolling.current = behavior === 'smooth'
    end.current?.scrollIntoView({ block: 'end', behavior })
    scrollInitialized.current = true
  }, [session.turns.length, answer.isPending, resultReady, transcribing])

  const submit = (variables: NonNullable<typeof answer.variables>) => {
    answer.mutate(variables)
  }
  const send = (reply: DiagnosticAnswer, display: string) => {
    if (!currentTurnId || answer.isPending) return
    audio.stop()
    submit({ turnId: currentTurnId, answer: reply, display })
  }
  // A session refresh can already contain the accepted student turn while
  // the submission is still pending. Only show its optimistic bubble until
  // the session moves past the question being answered.
  const pending =
    answer.isPending && answer.variables?.turnId === currentTurnId
      ? answer.variables
      : undefined
  // A failed answer belongs to its own turn: once the session moved on, the
  // old error and its retry are not offered for the new question.
  const failed =
    answer.error && answer.variables?.turnId === currentTurnId
      ? answer.error
      : null

  return (
    <main
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="dialog-title"
      onScroll={(event) => {
        if (autoScrolling.current) return
        const container = event.currentTarget
        followEnd.current =
          container.scrollHeight -
            container.scrollTop -
            container.clientHeight <
          96
      }}
    >
      {courseUpdated && (
        <Toast
          message={t('geek.courseUpdated')}
          closeLabel={t('geek.closeNotice')}
          onClose={closeNotice}
        />
      )}
      <Heading as="h1" id="dialog-title" className={styles.visuallyHidden}>
        {t('geek.title')}
      </Heading>
      <ol className={styles.turns}>
        {session.turns.map((turn) => (
          <TurnItem key={turn.id} turn={turn} audio={audio} muted={capturing} />
        ))}
        {pending && (
          <li className={styles.student}>
            <Text as="p" className={styles.bubble}>
              {pending.display}
            </Text>
          </li>
        )}
        {transcribing && (
          <li className={styles.student}>
            <Thinking label={t('geek.transcribing')} student />
          </li>
        )}
        {pending && (
          <li className={styles.geek}>
            <Thinking label={t('geek.thinking')} />
          </li>
        )}
      </ol>
      {refreshFailed && (
        <div className={styles.error} role="alert">
          <Text as="p">{t('geek.refreshError')}</Text>
          <Button size="2" variant="soft" onClick={onRefresh}>
            {t('trainer.retry')}
          </Button>
        </div>
      )}
      {session.status === 'completed' ? (
        <>
          {resultReady && (
            <ResultCard turns={session.turns} onResult={onResult} />
          )}
          <ComposerSpacer />
        </>
      ) : session.status === 'cancelled' ? (
        <>
          <div className={styles.footer} role="alert">
            <Text as="p">{t('geek.cancelled')}</Text>
            <Button size="3" variant="soft" onClick={onBack}>
              {t('home.back')}
            </Button>
          </div>
          <ComposerSpacer />
        </>
      ) : (
        <Composer
          key={currentTurnId}
          turnId={currentTurnId ?? ''}
          busy={answer.isPending}
          error={failed}
          onSend={send}
          // The retry repeats the failed request exactly: its turn and key.
          onRetry={() => {
            if (answer.variables && !answer.isPending) submit(answer.variables)
          }}
          onBeforeRecord={audio.stop}
          onCapture={setCapturing}
          onTranscription={setTranscribing}
        />
      )}
      <div ref={end} aria-hidden="true" />
    </main>
  )
}

// Keep the conversation's bottom spacing when the answer field is gone.
function ComposerSpacer() {
  return (
    <div
      className={`${styles.composer} ${styles.composerSpacer}`}
      aria-hidden="true"
    >
      <div className={styles.field}>
        <div className={styles.input} />
        <span className={styles.round} />
      </div>
    </div>
  )
}

// The way to the result: rises in with a comet running once around its border.
// The button takes the focus, so the keyboard and screen readers land on it.
function ResultCard({
  turns,
  onResult,
}: {
  turns: Turn[]
  onResult: () => void
}) {
  const { t } = useTranslation()
  const button = useRef<HTMLButtonElement>(null)
  useEffect(() => button.current?.focus({ preventScroll: true }), [])
  const topics = new Set(
    turns
      .filter((turn) => turn.kind === 'question' && turn.nodeId)
      .map((turn) => turn.nodeId),
  ).size
  return (
    <section className={styles.resultCard} aria-labelledby="result-card-title">
      <span className={styles.resultBadge} aria-hidden="true">
        <svg viewBox="0 0 52 52">
          <circle className={styles.resultRing} cx="26" cy="26" r="23" />
          <path className={styles.resultCheck} d="M16 27l7 7 13-15" />
        </svg>
      </span>
      <div className={styles.resultText}>
        <Heading as="h2" size="5" id="result-card-title">
          {t('geek.resultCard.title')}
        </Heading>
        <Text as="p" className={styles.resultLead}>
          {topics
            ? t('geek.resultCard.topics', { count: topics })
            : t('geek.resultCard.ready')}
        </Text>
      </div>
      <Button
        ref={button}
        size="3"
        className={styles.resultButton}
        onClick={onResult}
      >
        {t('geek.resultCard.open')}
        <ArrowRight size={18} aria-hidden="true" />
      </Button>
    </section>
  )
}

function TurnItem({
  turn,
  audio,
  muted,
}: {
  turn: Turn
  audio: ReturnType<typeof useTurnAudio>
  muted: boolean
}) {
  const { t } = useTranslation()
  if (turn.speaker === 'student')
    return (
      <li className={styles.student}>
        <Text as="p" className={styles.bubble}>
          {turn.source === 'dont_know' ? t('geek.dontKnowAnswer') : turn.text}
        </Text>
      </li>
    )
  return (
    <li className={styles.geek}>
      <div className={styles.geekHead}>
        <Text weight="bold">{t('geek.speaker')}</Text>
        {!turn.meta && turn.kind !== 'closing' && turn.kind !== 'answer' && (
          <Text size="2" className={styles.kind}>
            {t(`geek.kind.${turn.kind}`)}
          </Text>
        )}
        <ListenButton
          turn={turn}
          state={audio.state}
          audio={audio}
          disabled={muted}
        />
      </div>
      <MarkdownContent text={turn.text} className={styles.geekText} />
    </li>
  )
}

function ListenButton({
  turn,
  state,
  audio,
  disabled,
}: {
  turn: Turn
  state: TurnAudioState
  audio: ReturnType<typeof useTurnAudio>
  disabled: boolean
}) {
  const { t } = useTranslation()
  const mine = state.turnId === turn.id ? state.status : 'idle'
  const label =
    mine === 'playing'
      ? t('geek.stopListening')
      : mine === 'loading'
        ? t('geek.audioLoading')
        : mine === 'error'
          ? t('geek.audioError')
          : t('geek.listen')
  return (
    <button
      type="button"
      className={styles.listen}
      data-state={mine}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={() =>
        mine === 'playing' || mine === 'loading'
          ? audio.stop()
          : void audio.play(turn)
      }
    >
      {mine === 'loading' ? (
        <LoaderCircle size={16} className={styles.spin} aria-hidden="true" />
      ) : mine === 'playing' ? (
        <Square size={12} fill="currentColor" aria-hidden="true" />
      ) : mine === 'error' ? (
        <CircleAlert size={16} aria-hidden="true" />
      ) : (
        <Volume2 size={16} aria-hidden="true" />
      )}
    </button>
  )
}

function errorText(error: Error | null, t: (key: string) => string) {
  if (!error) return ''
  if (error instanceof DiagnosticApiError) {
    if (error.status === 409) return t('geek.staleTurn')
    if (error.status === 503 || error.code === 'TIMEOUT')
      return t('geek.modelUnavailable')
  }
  return t('geek.answerError')
}

// The answer field: text or a spoken answer sent as soon as it is transcribed.
function Composer({
  turnId,
  busy,
  error,
  onSend,
  onRetry,
  onBeforeRecord,
  onCapture,
  onTranscription,
}: {
  turnId: string
  busy: boolean
  error: Error | null
  onSend: (answer: DiagnosticAnswer, display: string) => void
  onRetry: () => void
  // Stop playback synchronously before requesting the microphone.
  onBeforeRecord: () => void
  // Reports actual microphone or transcript activity, including failed starts.
  onCapture: (active: boolean) => void
  onTranscription: (active: boolean) => void
}) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const submittedText = useRef<string | null>(null)
  const voice = useVoiceAnswer(turnId, (transcript) =>
    onSend({ kind: 'voice', transcriptionId: transcript.id }, transcript.text),
  )
  const recording = voice.recording || voice.requesting
  const capturing = recording || voice.preparing || voice.transcribing
  useEffect(() => {
    onCapture(capturing)
  }, [capturing, onCapture])
  useEffect(() => () => onCapture(false), [onCapture])
  useEffect(() => {
    onTranscription(voice.transcribing)
  }, [voice.transcribing, onTranscription])
  useEffect(() => () => onTranscription(false), [onTranscription])
  useEffect(() => {
    if (error && submittedText.current !== null) {
      setText(submittedText.current)
      submittedText.current = null
    }
  }, [error])
  const stale = error instanceof DiagnosticApiError && error.status === 409
  const seconds = `${Math.floor(voice.elapsedSeconds / 60)}:${String(voice.elapsedSeconds % 60).padStart(2, '0')}`
  const sendText = () => {
    const value = text.trim()
    if (value) {
      submittedText.current = value
      setText('')
      onSend({ kind: 'text', text: value }, value)
    }
  }

  return (
    <div className={styles.composer}>
      {error && !busy && (
        <div className={styles.error} role="alert">
          <Text as="p">{errorText(error, t)}</Text>
          {!stale && (
            <Button size="2" variant="soft" onClick={onRetry}>
              {t('geek.retryAnswer')}
            </Button>
          )}
        </div>
      )}
      {voice.error && (
        <Text as="p" role="alert" className={styles.voiceError}>
          {t(
            voice.error === 'denied' ? 'geek.micDenied' : 'geek.micUnavailable',
          )}
        </Text>
      )}
      {voice.transcriptionError && (
        <div className={styles.error} role="alert">
          <Text as="p">
            {t(
              voice.transcriptionError === 'no_speech'
                ? 'geek.noSpeech'
                : 'geek.transcriptionError',
            )}
          </Text>
          {voice.transcriptionError === 'transcription' && (
            <Button size="2" variant="soft" onClick={voice.retryTranscription}>
              {t('geek.retryTranscription')}
            </Button>
          )}
        </div>
      )}
      {recording ? (
        <div className={styles.recording}>
          <Text role="timer" className={styles.timer}>
            {seconds}
          </Text>
          <div className={styles.wave} aria-hidden="true">
            <LiveWaveform stream={voice.stream} height={32} />
          </div>
          <Text className={styles.visuallyHidden} role="status">
            {t('geek.recording')}
          </Text>
          <button
            type="button"
            className={styles.round}
            data-tone="stop"
            aria-label={t('geek.stopRecording')}
            onClick={voice.stop}
          >
            <Square size={16} fill="currentColor" aria-hidden="true" />
          </button>
        </div>
      ) : (
        <form
          className={styles.field}
          onSubmit={(event) => {
            event.preventDefault()
            sendText()
          }}
        >
          <TextArea
            className={styles.input}
            aria-label={t('geek.answerLabel')}
            placeholder={t('geek.answerPlaceholder')}
            value={text}
            disabled={busy || voice.preparing || voice.transcribing}
            maxLength={4000}
            rows={1}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault()
                sendText()
              }
            }}
          />
          <button
            type={text.trim() ? 'submit' : 'button'}
            className={styles.round}
            aria-label={text.trim() ? t('geek.send') : t('geek.record')}
            title={text.trim() ? t('geek.send') : t('geek.record')}
            disabled={busy || voice.preparing || voice.transcribing}
            onClick={
              text.trim()
                ? undefined
                : () => {
                    // Geek's voice must not end up in the recording.
                    onBeforeRecord()
                    void voice.start()
                  }
            }
          >
            {text.trim() ? (
              <Send size={18} aria-hidden="true" />
            ) : (
              <Mic size={20} aria-hidden="true" />
            )}
          </button>
        </form>
      )}
    </div>
  )
}

// Geek's typing: three dots in a bubble. Keep the status label for screen readers.
function Thinking({
  label,
  student = false,
}: {
  label: string
  student?: boolean
}) {
  return (
    <Text
      role="status"
      className={student ? styles.studentThinking : styles.thinking}
    >
      <span
        className={styles.dots}
        data-tone={student ? 'student' : undefined}
        aria-hidden="true"
      >
        <span />
        <span />
        <span />
      </span>
      <span className={styles.visuallyHidden}>{label}</span>
    </Text>
  )
}
