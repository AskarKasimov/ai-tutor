import { Button, Heading, Text } from '@radix-ui/themes'
import { CircleAlert, LoaderCircle, Mic, RotateCcw, Square } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type { PropsWithChildren } from 'react'
import { useTranslation } from 'react-i18next'
import { useTrainerPrototype } from '../data/use-trainer-prototype'
import { isMockApi } from '../data/api-fetch'
import type { SessionAnswer, SessionTask } from '../shared/domain'
import styles from './index.module.scss'

const barHeights = [18, 30, 45, 26, 61, 82, 53, 30, 68, 42, 25, 52, 72, 33, 48, 27, 59, 38, 20]

function VoiceIllustration({ recording }: { recording: boolean }) {
  return <div className={`${styles.waveform} ${recording ? styles.waveformActive : ''}`} aria-hidden="true"><div className={styles.waveBars}>{barHeights.map((height, i) => <span key={i} style={recording ? { height } : undefined} />)}</div></div>
}

function MicrophoneIllustration() {
  return <div className={styles.microphone} aria-hidden="true"><Mic size={40} /></div>
}

export function TrainerAnswer({ task, setBusy, onSave, saving, saveError }: {
  task: SessionTask; setBusy: (busy: boolean) => void
  onSave: (answer: SessionAnswer, audioBlob?: Blob) => Promise<void>
  saving: boolean; saveError: boolean
}) {
  const { t } = useTranslation()
  const assessmentTask = { question: t(task.questionKey), options: task.optionKeys.map((key) => t(key)), voiceInstruction: t(task.instructionKey) }
  const voice = useTrainerPrototype(assessmentTask, [assessmentTask.question, ...assessmentTask.options].join(' '))
  const stage = voice.stage
  const recording = stage === 'recording'
  const waiting = stage === 'permission'
  const processing = stage === 'processing' || stage === 'grading'
  const error = stage === 'error'
  const busy = recording || waiting || processing || saving || stage === 'result'
  const submitted = useRef(false)
  const seconds = `${Math.floor(voice.seconds / 60).toString().padStart(2, '0')}:${(voice.seconds % 60).toString().padStart(2, '0')}`
  function save() {
    if (!voice.assessment) return Promise.resolve()
    return onSave({ assignmentId: task.assignmentId, transcript: voice.transcript, assessment: voice.assessment, submittedAt: Math.floor(Date.now() / 1000) }, voice.audioBlob)
  }
  useEffect(() => { setBusy(busy); return () => setBusy(false) }, [busy, setBusy])
  useEffect(() => {
    if (stage !== 'result' || !voice.assessment || submitted.current) return
    submitted.current = true
    void onSave({ assignmentId: task.assignmentId, transcript: voice.transcript, assessment: voice.assessment, submittedAt: Math.floor(Date.now() / 1000) }, voice.audioBlob).catch(() => {})
  }, [stage, voice.assessment, voice.transcript, voice.audioBlob, task.assignmentId, onSave])

  const titleKey = recording ? 'recording' : stage === 'grading' ? 'grading' : processing ? 'processing' : waiting ? 'permission' : error ? (voice.transcript ? 'yourAnswer' : voice.error) : 'answer'
  const helpKey = recording ? 'recordingHelp' : stage === 'grading' ? 'gradingHelp' : processing ? 'processingHelp' : waiting ? 'permissionHelp' : error ? `${voice.error}Help` : 'answerHelp'
  return <>
    <div className={styles.repeatControl}>
      <Button variant="soft" className={styles.repeat} onClick={voice.speak} disabled={busy} title={isMockApi() ? t('mockApi.speechHint') : undefined}>
        {voice.loadingSpeech ? <LoaderCircle size={18} className={styles.spinner} aria-hidden="true" /> : voice.speaking ? <Square size={16} aria-hidden="true" /> : <RotateCcw size={18} aria-hidden="true" />}
        {t(voice.loadingSpeech ? 'trainer.cancelSpeech' : voice.speaking ? 'trainer.stopSpeech' : 'session.repeatQuestion')}
      </Button>
      {voice.speechError && <Text role="alert" as="p" className={styles.speechError}>{t(`trainer.${voice.speechError}`)}</Text>}
    </div>
    <aside className={`${styles.answerPanel} ${error ? styles.answerError : ''}`} aria-label={t('trainer.answerArea')}>
      <Text as="p" className={styles.eyebrow}>{t('trainer.yourAnswer')}</Text>
      <VoiceIllustration recording={recording} />
      <div aria-live="polite" aria-atomic="true" role={error ? 'alert' : undefined}>
        <Heading as="h2" className={styles.answerTitle}>{t(`trainer.${titleKey}`, { time: seconds })}</Heading>
        {voice.transcript ? <Text as="p" className={styles.transcript}>{voice.transcript}</Text> : <Text as="p" className={styles.instructions}>{t(`trainer.${helpKey}`)}</Text>}
        {error && voice.transcript && <Text as="p" className={styles.speechError}>{t(`trainer.${voice.error}Help`)}</Text>}
      </div>
      {recording ? <div className={styles.recordingTimer}><span />{seconds}</div> : processing || waiting || saving ? <div className={styles.processingIndicator}><LoaderCircle size={32} className={styles.spinner} aria-hidden="true" /></div> : error ? <div className={styles.processingIndicator}><CircleAlert size={32} aria-hidden="true" /></div> : <MicrophoneIllustration />}
      {stage === 'result' ? <div className={styles.saveState}><Text role={saveError ? 'alert' : 'status'}>{t(saveError ? 'session.saveError' : 'session.saving')}</Text>{saveError && <Button className={styles.primary} onClick={() => void save().catch(() => {})}>{t('session.retrySave')}</Button>}</div> : <Button className={styles.primary} disabled={processing || waiting} onClick={recording ? voice.stop : error && voice.transcript ? voice.retryAssessment : voice.start}>{t(`trainer.${recording ? 'stopRecording' : stage === 'grading' ? 'grading' : processing ? 'busy' : waiting ? 'allow' : error && voice.transcript ? 'retryAssessment' : error ? 'retry' : 'start'}`)}</Button>}
      <Text as="p" className={styles.microphoneStatus}>{t(recording ? 'session.recordingStatus' : error ? 'session.errorStatus' : processing ? 'session.processingStatus' : waiting ? 'trainer.allow' : 'session.microphoneReady')}</Text>
    </aside>
  </>
}

export function SavedAnswer({ answer, audioUrl, children }: PropsWithChildren<{ answer: SessionAnswer; audioUrl?: string }>) {
  const { t } = useTranslation()
  return <aside className={styles.answerPanel} aria-label={t('trainer.answerArea')}>
    <Text as="p" className={styles.eyebrow}>{t('session.result')}</Text>
    <div className={`${styles.score} ${answer.assessment.score === 0 ? styles.scoreIncorrect : answer.assessment.score === 1 ? styles.scorePartial : ''}`}><Text>{answer.assessment.score} / 2</Text><Text className={styles.verdict}>{t(`session.verdict.${answer.assessment.score}`)}</Text></div>
    <Heading as="h2" className={styles.answerTitle}>{t('trainer.yourAnswer')}</Heading>
    <Text as="p" className={styles.transcript}>{answer.transcript}</Text>
    {audioUrl && <div className={styles.audio}><Text as="p">{t('trainer.listenRecording')}</Text><audio controls src={audioUrl} aria-label={t('trainer.listenRecording')} /></div>}
    <Heading as="h2" className={styles.feedbackTitle}>{t('trainer.feedback')}</Heading>
    {answer.assessment.feedback.map((line, i) => <Text as="p" className={styles.feedbackLine} key={i}>{line}</Text>)}
    <div className={styles.resultActions}>{children}</div>
  </aside>
}
