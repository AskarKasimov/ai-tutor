import { Button, Heading, Text } from '@radix-ui/themes'
import { createFileRoute } from '@tanstack/react-router'
import { Check, GraduationCap, Slash } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../data/use-auth'
import { isMockApi } from '../data/api-fetch'
import { useTrainerSession } from '../data/use-trainer-session'
import type { TrainerSession } from '../shared/domain'
import { DiagnosticTrainer } from './-diagnostic-trainer'
import { AccountMenu } from './-account-menu'
import { TrainerAnswer, SavedAnswer } from './-trainer-answer'
import { SessionSummary } from './-session-summary'
import styles from './index.module.scss'

export const Route = createFileRoute('/')({ component: Trainer })

function Trainer() {
  return isMockApi() ? <DemoTrainer /> : <DiagnosticTrainer />
}

function DemoTrainer() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const training = useTrainerSession(user?.id)
  const mock = isMockApi()
  return <div data-trainer-app>
    <div className={`${styles.prototype} ${mock ? styles.mockLayout : ''}`}>
    <aside className={styles.navigation}>
      <div className={styles.brand}>
        <span className={styles.brandIcon} aria-hidden="true"><GraduationCap size={24} /></span>
        <Text>{t('trainer.title')}</Text>
      </div>
      <div className={styles.courseHeading}><Text as="p" className={styles.eyebrow}>{t('session.session')}</Text><Heading as="h2">{t('session.course')}</Heading></div>
      {training.query.data && <SessionProgress session={training.query.data} />}
    </aside>
    <div className={styles.content}>
      <header className={styles.header}>
        <div className={styles.breadcrumb}><Text>{t('session.practice')}</Text><span aria-hidden="true"><Slash size={14} /></span><Text weight="bold">{t('session.course')}</Text></div>
        <AccountMenu />
      </header>
      {user && training.query.isPending ? <main className={styles.notice} aria-live="polite"><Text>{t('session.loading')}</Text></main> : user && training.query.isError ? <main className={styles.notice}><Text role="alert">{t('session.loadError')}</Text><Button onClick={() => void training.query.refetch()}>{t('trainer.retry')}</Button></main> : training.query.data ? training.query.data.tasks.length ? <SessionScreen key={training.query.data.id} session={training.query.data} training={training} /> : <main className={styles.notice}><Text>{t('session.empty')}</Text></main> : <main className={styles.notice}><Text>{t('auth.checkingSession')}</Text></main>}
    </div>
    </div>
  </div>
}

function SessionProgress({ session }: { session: TrainerSession }) {
  const { t } = useTranslation()
  return <div className={styles.progress}>
    <Text as="p">{t('session.progress', { count: Math.min(session.answers.length + 1, 7), total: 7 })}</Text>
    <div className={styles.progressTrack} role="progressbar" aria-label={t('session.progressLabel')} aria-valuemin={0} aria-valuemax={7} aria-valuenow={session.answers.length}>
      <span className={styles.progressFill} style={{ width: `${session.answers.length / 7 * 100}%` }} />
    </div>
  </div>
}

function SessionScreen({ session, training }: { session: TrainerSession; training: ReturnType<typeof useTrainerSession> }) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState(session.tasks[0].assignmentId)
  const [summary, setSummary] = useState(false)
  const [busy, setBusy] = useState(false)
  const [reviewing, setReviewing] = useState(false)
  const selected = session.tasks.find((task) => task.assignmentId === selection) ?? session.tasks[0]
  const answer = session.answers.find((item) => item.assignmentId === selection)
  const audioUrl = training.audioUrl(selection)
  const index = session.tasks.indexOf(selected)
  const blocked = busy || training.save.isPending || training.restart.isPending
  const current = session.currentAssignmentId
  function select(id: string) { setSelection(id); setSummary(false); setReviewing(true); training.save.reset() }
  function advance() { if (current) { select(current); setReviewing(false) } else setSummary(true) }

  return <>
    <nav className={styles.assignmentNavigation} aria-label={t('session.assignments')}>
      {session.tasks.map((task, position) => {
        const completed = session.answers.some((item) => item.assignmentId === task.assignmentId)
        const active = selection === task.assignmentId && !summary
        return <button key={task.assignmentId} className={`${styles.assignment} ${active ? styles.assignmentActive : ''} ${completed ? styles.assignmentCompleted : ''}`} aria-current={active ? 'step' : undefined}
          disabled={blocked || (!completed && task.assignmentId !== current)} onClick={() => select(task.assignmentId)}>
          <span className={styles.assignmentNumber}>{String(position + 1).padStart(2, '0')}</span><span>{t('session.assignment', { number: position + 1 })}</span>
          {completed && <span className={styles.completedMark} aria-hidden="true"><Check size={14} /></span>}
        </button>
      })}
    </nav>
    {summary ? <SessionSummary session={session} pending={training.restart.isPending} error={training.restart.isError} onRestart={() => training.restart.mutate()} onReview={select} /> : <main className={styles.workspace}>
      <section className={styles.question}>
        <div className={styles.questionToolbar}><Text className={styles.questionBadge}>{t('session.questionNumber', { number: String(index + 1).padStart(2, '0') })}</Text>
          <span className={styles.questionCount}><span className={styles.questionDot} aria-hidden="true" />{String(index + 1).padStart(2, '0')} / 07</span>
        </div>
        <Heading as="h1" className={styles.questionTitle}>{t(selected.questionKey)}</Heading>
        <section aria-labelledby="answer-options-title" className={styles.options}>
          <Heading as="h2" id="answer-options-title" className={styles.eyebrow}>{t('trainer.optionsTitle')}</Heading>
          <ol className={styles.optionList}>{selected.optionKeys.map((key, position) => <li key={key} className={styles.option}><span className={styles.optionLetter} aria-hidden="true">{'ABCD'[position]}</span><Text>{t(key)}</Text></li>)}</ol>
        </section>
        {answer && <section aria-labelledby="saved-answer-title" className={styles.savedAnswer}>
          <Heading as="h2" id="saved-answer-title" className={styles.eyebrow}>{t('trainer.yourAnswer')}</Heading>
          <Text as="p" className={styles.transcript}>{answer.transcript}</Text>
          {audioUrl && <div className={styles.audio}><Text as="p">{t('trainer.listenRecording')}</Text><audio controls src={audioUrl} aria-label={t('trainer.listenRecording')} /></div>}
        </section>}
      </section>
      {answer ? <SavedAnswer answer={answer}>
        <Button className={styles.primary} onClick={advance}>{t(current && reviewing ? 'session.returnCurrent' : current ? 'session.next' : 'session.viewSummary')}</Button>
      </SavedAnswer> : <TrainerAnswer key={`${session.id}/${selection}`} task={selected} setBusy={setBusy} saving={training.save.isPending} saveError={training.save.isError}
        onSave={(result, audioBlob) => training.save.mutateAsync({ sessionId: session.id, answer: result, audioBlob }).then(() => undefined)} />}
    </main>}
  </>
}
