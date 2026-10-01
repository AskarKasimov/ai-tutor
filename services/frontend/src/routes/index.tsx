import { Button, Heading, Text } from '@radix-ui/themes'
import { createFileRoute } from '@tanstack/react-router'
import { ArrowRight, Check, CircleAlert, LoaderCircle, Mic, RotateCcw, Square, Volume2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useTrainerPrototype } from '../data/use-trainer-prototype'
import styles from './index.module.scss'
import { AuthModal } from './-auth-modal'

export const Route = createFileRoute('/')({ component: TrainerPrototype })

function TrainerPrototype() {
  const { t } = useTranslation()
  const voice = useTrainerPrototype(t('trainer.question'))
  const { stage } = voice
  const active = stage === 'recording'
  const processing = stage === 'processing'
  const result = stage === 'result'
  const error = stage === 'error'
  const waiting = stage === 'permission'
  const time = `${Math.floor(voice.seconds / 60).toString().padStart(2, '0')}:${(voice.seconds % 60).toString().padStart(2, '0')}`

  return (
    <div className={styles.prototype} data-trainer-app>
      <header className={styles.header}>
        <img src="/assets/voice-trainer/layer-1.svg" alt="" width="36" height="36" />
        <Text className={styles.brand}>{t('trainer.title')}</Text>
        <AuthModal />
      </header>
      <main className={styles.workspace}>
        <section className={styles.question}>
          <Text as="p" className={styles.eyebrow}>{t('trainer.course')}</Text>
          <Heading as="h1" className={styles.questionTitle}>{t('trainer.question')}</Heading>
        </section>
        <div className={styles.questionControl}>
          <Button className={styles.repeat} variant="soft" onClick={voice.speak} disabled={active || processing || waiting}>
            {voice.loadingSpeech ? <LoaderCircle size={18} className={styles.spinner} /> : voice.speaking ? <Square size={17} fill="currentColor" /> : <RotateCcw size={18} />}
            {t(voice.loadingSpeech ? 'trainer.cancelSpeech' : voice.speaking ? 'trainer.stopSpeech' : 'trainer.repeat')}
          </Button>
          {voice.speechError && <Text as="p" role="alert" className={styles.speechError}>{t(`trainer.${voice.speechError}`)}</Text>}
        </div>
        <section className={styles.card} aria-label={t('trainer.answerArea')}>
          <div className={`${styles.answer} ${error ? styles.answerError : ''}`}>
            <div aria-live="polite" aria-atomic="true" role={error ? 'alert' : undefined}>
              <Heading as="h2" className={styles.answerTitle}>
                {active ? t('trainer.recording', { time }) : t(`trainer.${result ? (voice.example ? 'exampleTitle' : 'yourAnswer') : processing ? 'processing' : waiting ? 'permission' : error ? voice.error : 'answer'}`)}
              </Heading>
              {result ? <Text as="p" className={styles.transcript}>{voice.example ? t('trainer.exampleAnswer') : voice.transcript}</Text> : (
                <Text as="p" className={styles.instructions}>
                  {t(`trainer.${active ? 'recordingHelp' : processing ? 'processingHelp' : waiting ? 'permissionHelp' : error ? `${voice.error}Help` : 'answerHelp'}`)}
                </Text>
              )}
            </div>
            {active && <div className={styles.waveform} aria-hidden="true">{Array.from({ length: 46 }, (_, i) => <span key={i} />)}</div>}
            {processing && <div className={styles.processingIndicator}><LoaderCircle size={26} className={styles.spinner} /><Text>{t('trainer.processingHint')}</Text></div>}
            {error && <CircleAlert size={28} className={styles.errorIcon} aria-hidden="true" />}
            {result && voice.audioUrl && <div className={styles.audio}><Text as="p">{t('trainer.listenRecording')}</Text><audio controls src={voice.audioUrl} aria-label={t('trainer.listenRecording')} /></div>}
          </div>
          <div className={styles.cardFooter}>
            {result ? <Text className={styles.success}><Check size={17} />{t(voice.example ? 'trainer.finished' : 'trainer.transcriptionFinished')}</Text> : (
              <Button className={styles.primary} disabled={processing || waiting} onClick={active ? voice.stop : voice.start}>
                {processing || waiting ? <LoaderCircle size={18} className={styles.spinner} /> : active ? <Square size={15} fill="currentColor" /> : <Mic size={18} />}
                {t(`trainer.${active ? 'stopRecording' : processing ? 'busy' : waiting ? 'allow' : error ? 'retry' : 'start'}`)}
              </Button>
            )}
            {(stage === 'ready' || error) && <Button variant="ghost" className={styles.preview} onClick={voice.showExample}>{t('trainer.preview')}<ArrowRight size={15} /></Button>}
          </div>
        </section>
        <aside className={styles.sidebar} aria-live="polite">
          {result && !voice.example ? <>
            <Heading as="h2" className={styles.sideTitle}>{t('trainer.transcriptionReady')}</Heading>
            <Text as="p" className={styles.sideText}>{t('trainer.noAssessment')}</Text>
            <Button className={`${styles.primary} ${styles.again}`} onClick={voice.reset}><RotateCcw size={17} />{t('trainer.again')}</Button>
          </> : result ? <>
            <Heading as="h2" className={styles.sideTitle}>{t('trainer.score')}</Heading>
            <div className={styles.score}><Text>2 / 2</Text><Check size={26} aria-hidden="true" /></div>
            <Heading as="h2" className={styles.feedbackTitle}>{t('trainer.feedback')}</Heading>
            <Text as="p" className={styles.sideText}>{t('trainer.exampleFeedback')}</Text>
            <Text as="p" className={styles.demoNote}>{t('trainer.exampleNote')}</Text>
            <Button className={`${styles.primary} ${styles.again}`} onClick={voice.reset}><RotateCcw size={17} />{t('trainer.again')}</Button>
          </> : <>
            <Heading as="h2" className={styles.sideTitle}>{t('trainer.voiceAnswer')}</Heading>
            <Text as="p" className={styles.sideText}>{t('trainer.sideHelp')}</Text>
            <div className={styles.sideHint}><Text as="p">{t('trainer.sideHint')}</Text></div>
          </>}
        </aside>
        <footer className={styles.prototypeNote}><Volume2 size={14} aria-hidden="true" /><Text>{t('trainer.prototypeNote')}</Text></footer>
      </main>
    </div>
  )
}
