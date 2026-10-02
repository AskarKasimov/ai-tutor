import { Button, Heading, Text } from '@radix-ui/themes'
import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowRight, Check, CircleAlert, LoaderCircle, Mic, RotateCcw, Square, Volume2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { TaskGenerationApiError } from '../data/task-generation-api'
import { useGenerateTrainingTasks, useOutcomesQuery } from '../data/use-training-tasks'
import { useTrainerPrototype } from '../data/use-trainer-prototype'
import type { GeneratedTask } from '../shared/domain'
import styles from './training.module.scss'
import { AuthModal } from './-auth-modal'

export const Route = createFileRoute('/training')({ component: TrainingGenerator })

const COUNTS = [1, 2, 3, 4, 5] as const

function TrainingGenerator() {
  const { t } = useTranslation()
  const outcomes = useOutcomesQuery()
  const generate = useGenerateTrainingTasks()
  const [outcomeId, setOutcomeId] = useState('')
  const [count, setCount] = useState(3)
  const [taskIndex, setTaskIndex] = useState(0)

  const list = outcomes.data ?? []
  const activeOutcomeId = outcomeId || list[0]?.id || ''
  const activeOutcome = list.find((outcome) => outcome.id === activeOutcomeId)
  const generated = generate.data
  const tasks = generated?.tasks ?? []
  const selected = tasks[taskIndex]
  const generateError = generate.isError ? (generate.error instanceof TaskGenerationApiError && generate.error.status === 401 ? 'unauthorized' : 'generateError') : ''

  function run() {
    if (!activeOutcomeId || activeOutcome?.taskCount === 0 || generate.isPending) return
    setTaskIndex(0)
    generate.mutate({ outcomeId: activeOutcomeId, count })
  }

  return (
    <div className={styles.page} data-trainer-app>
      <header className={styles.header}>
        <img src="/assets/voice-trainer/layer-1.svg" alt="" width="36" height="36" />
        <Text className={styles.brand}>{t('training.title')}</Text>
        <Link to="/" className={styles.back}>{t('training.back')}</Link>
        <AuthModal />
      </header>
      <main className={styles.workspace}>
        <section className={styles.setup} aria-label={t('training.heading')}>
          <Heading as="h1" className={styles.heading}>{t('training.heading')}</Heading>
          <Text as="p" className={styles.intro}>{t('training.intro')}</Text>
          {outcomes.isPending ? <Text as="p" className={styles.status}><LoaderCircle size={16} className={styles.spinner} />{t('training.outcomesLoading')}</Text>
            : outcomes.isError ? <div className={styles.statusError}><Text as="p" role="alert">{t('training.outcomesError')}</Text><Button variant="soft" onClick={() => outcomes.refetch()}>{t('training.retry')}</Button></div>
              : list.length === 0 ? <Text as="p" className={styles.status}>{t('training.outcomesEmpty')}</Text>
                : <>
                  <label className={styles.field}>{t('training.topic')}
                    <select value={activeOutcomeId} disabled={generate.isPending} onChange={(event) => { setOutcomeId(event.target.value); setTaskIndex(0); generate.reset() }}>
                      {list.map((outcome) => <option key={outcome.id} value={outcome.id} disabled={outcome.taskCount === 0}>
                        {outcome.competencyName} · {outcome.constituentName} · {outcome.name}{outcome.taskCount === 0 ? ` · ${t('training.noSamples')}` : ''}
                      </option>)}
                    </select>
                  </label>
                  {activeOutcome?.taskCount === 0 && <Text as="p" className={styles.status}>{t('training.noSamplesHelp')}</Text>}
                  <label className={styles.field}>{t('training.count')}
                    <select value={count} disabled={generate.isPending} onChange={(event) => { setCount(Number(event.target.value)); generate.reset() }}>
                      {COUNTS.map((value) => <option key={value} value={value}>{value}</option>)}
                    </select>
                  </label>
                  <Button className={styles.generate} onClick={run} disabled={generate.isPending || !activeOutcomeId || activeOutcome?.taskCount === 0}>
                    {generate.isPending ? <LoaderCircle size={18} className={styles.spinner} /> : <ArrowRight size={18} />}
                    {t(generate.isPending ? 'training.generating' : 'training.generate')}
                  </Button>
                  {generateError && <Text as="p" role="alert" className={styles.error}>{t(`training.${generateError}`)}</Text>}
                </>}
        </section>
        {generated && selected && (
          <TaskPractice key={taskIndex} task={selected} index={taskIndex} total={tasks.length} outcomeName={generated.outcomeName}
            onNext={() => setTaskIndex((value) => value + 1)} />
        )}
      </main>
    </div>
  )
}

function TaskPractice({ task, index, total, outcomeName, onNext }: { task: GeneratedTask; index: number; total: number; outcomeName: string; onNext: () => void }) {
  const { t } = useTranslation()
  const voice = useTrainerPrototype({
    question: task.question,
    options: task.options,
    voiceInstruction: task.voiceInstruction,
    correctAnswer: task.criteria,
  })
  const { stage } = voice
  const active = stage === 'recording'
  const processing = stage === 'processing' || stage === 'grading'
  const result = stage === 'result'
  const error = stage === 'error'
  const waiting = stage === 'permission'
  const last = index + 1 >= total
  const time = `${Math.floor(voice.seconds / 60).toString().padStart(2, '0')}:${(voice.seconds % 60).toString().padStart(2, '0')}`

  return (
    <section className={styles.practice} aria-label={t('training.tasksTitle')}>
      <div className={styles.practiceHeader}>
        <Text as="p" className={styles.progress}>{t('training.taskProgress', { index: index + 1, total })}</Text>
        <Text as="p" className={styles.outcome}>{t('training.sourceOutcome', { name: outcomeName })}</Text>
      </div>
      <Heading as="h2" className={styles.question}>{task.question}</Heading>
      <div className={styles.instructionRow}>
        <Button className={styles.repeat} variant="soft" onClick={voice.speak} disabled={active || processing || waiting}>
          {voice.loadingSpeech ? <LoaderCircle size={18} className={styles.spinner} /> : voice.speaking ? <Square size={17} fill="currentColor" /> : <Volume2 size={18} />}
          {t(voice.loadingSpeech ? 'trainer.cancelSpeech' : voice.speaking ? 'trainer.stopSpeech' : 'trainer.playInstruction')}
        </Button>
        {voice.speechError && <Text as="p" role="alert" className={styles.error}>{t(`trainer.${voice.speechError}`)}</Text>}
      </div>
      {task.options.length > 0 && (
        <section className={styles.options} aria-label={t('trainer.optionsTitle')}>
          <Heading as="h3" className={styles.optionsTitle}>{t('trainer.optionsTitle')}</Heading>
          <ol className={styles.optionList}>{task.options.map((option) => <li className={styles.option} key={option}><Text>{option}</Text></li>)}</ol>
        </section>
      )}
      <div className={`${styles.answer} ${error ? styles.answerError : ''}`}>
        <div aria-live="polite" aria-atomic="true" role={error ? 'alert' : undefined}>
          <Heading as="h3" className={styles.answerTitle}>
            {active ? t('trainer.recording', { time }) : t(`trainer.${result ? 'yourAnswer' : stage === 'grading' ? 'grading' : processing ? 'processing' : waiting ? 'permission' : error && voice.transcript ? 'yourAnswer' : error ? voice.error : 'answer'}`)}
          </Heading>
          {result || (error && voice.transcript) ? <Text as="p" className={styles.transcript}>{voice.transcript}</Text>
            : error ? <Text as="p" className={styles.instructions}>{t(`trainer.${voice.error}Help`)}</Text>
              : <Text as="p" className={styles.instructions}>{t(active ? 'trainer.recordingHelp' : processing || waiting ? 'trainer.processingHelp' : 'trainer.answerHelp')}</Text>}
        </div>
        {processing && <div className={styles.processingIndicator}><LoaderCircle size={22} className={styles.spinner} /><Text>{t(stage === 'grading' ? 'trainer.gradingHint' : 'trainer.processingHint')}</Text></div>}
        {error && <CircleAlert size={26} className={styles.errorIcon} aria-hidden="true" />}
      </div>
      {result && voice.assessment && (
        <div className={styles.result}>
          <div className={`${styles.score} ${voice.assessment.score === 0 ? styles.scoreIncorrect : voice.assessment.score === 1 ? styles.scorePartial : ''}`}>
            <Text>{voice.assessment.score} / 2</Text>
            {voice.assessment.score === 2 ? <Check size={24} aria-hidden="true" /> : <CircleAlert size={24} aria-hidden="true" />}
          </div>
          <div className={styles.feedback}>
            <Heading as="h3" className={styles.feedbackTitle}>{t('trainer.feedback')}</Heading>
            {voice.assessment.feedback.map((line, position) => <Text as="p" className={styles.feedbackLine} key={position}>{line}</Text>)}
          </div>
        </div>
      )}
      <div className={styles.practiceFooter}>
        {result ? (last ? <Text className={styles.done}><Check size={16} />{t('training.finish')}</Text>
          : <Button className={styles.next} onClick={onNext}><ArrowRight size={16} />{t('training.next')}</Button>)
          : <Button className={styles.record} disabled={processing || waiting} onClick={active ? voice.stop : error && voice.transcript ? voice.retryAssessment : voice.start}>
            {processing || waiting ? <LoaderCircle size={18} className={styles.spinner} /> : active ? <Square size={15} fill="currentColor" /> : error && voice.transcript ? <RotateCcw size={18} /> : <Mic size={18} />}
            {t(`trainer.${active ? 'stopRecording' : stage === 'grading' ? 'grading' : processing ? 'busy' : waiting ? 'allow' : error && voice.transcript ? 'retryAssessment' : error ? 'retry' : 'start'}`)}
          </Button>}
      </div>
    </section>
  )
}
