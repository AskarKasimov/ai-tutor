import { Button, Card, Heading, Text } from '@radix-ui/themes'
import {
  GraduationCap,
  Mic,
  RotateCcw,
  Sparkles,
  Target,
  Zap,
} from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import {
  useLearningStateQuery,
  useSubjectsQuery,
} from '@/features/subject-selection'
import { subjectQueryKeys, type Subject } from '@/entities/subject'
import { LearningPlanEntry } from './learning-plan'
import { DiagnosticTrainer } from './diagnostic-trainer'
import { AccountMenu } from '@/features/auth'
import styles from './learning-home.module.scss'

type Intent = {
  apiBase: string
  userId: string
  subjectId: string
  sessionId: string
  mode: 'diagnostic' | 'training'
  diagnosticId?: string
}
const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
function readIntent(userId: string): Intent | null {
  try {
    const raw = sessionStorage.getItem('ai-tutor:learning-home')
    const value = JSON.parse(raw ?? 'null') as Intent | null
    const valid =
      value?.userId === userId &&
      value.apiBase === apiBase &&
      (value.mode === 'diagnostic' || value.mode === 'training')
    if (!value || !valid) return null
    if (value.mode === 'diagnostic' && !value.sessionId) {
      // A pending create is not a resumable session. Require a new explicit
      // start click, while diagnostic identity storage retains its idempotency keys.
      sessionStorage.removeItem('ai-tutor:learning-home')
      return null
    }
    return value
  } catch {
    return null
  }
}

export function LearningHome() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const userId = user?.id ?? ''
  const subjectsQuery = useSubjectsQuery(userId)
  const [intent, setIntent] = useState<Intent | null>(() =>
    userId ? readIntent(userId) : null,
  )
  const subjects = subjectsQuery.data ?? []
  // Students study the single subject the teacher manages: the first one.
  const selected =
    subjects.find((subject) => subject.id === intent?.subjectId) ?? subjects[0]
  const learning = useLearningStateQuery(userId, selected?.id ?? '')

  useEffect(() => {
    if (
      intent &&
      !subjectsQuery.isPending &&
      subjects.length &&
      !subjects.some((s) => s.id === intent.subjectId)
    ) {
      sessionStorage.removeItem('ai-tutor:learning-home')
      setIntent(null)
    }
  }, [intent, subjects, subjectsQuery.isPending])

  function openDiagnostic(
    subject: Subject,
    sessionId: string | undefined,
    startNew: boolean,
  ) {
    if (!userId) return
    const next: Intent = {
      apiBase,
      userId,
      subjectId: subject.id,
      sessionId: sessionId ?? '',
      mode: 'diagnostic',
    }
    if (startNew) next.sessionId = ''
    sessionStorage.setItem('ai-tutor:learning-home', JSON.stringify(next))
    setIntent(next)
  }
  function back() {
    sessionStorage.removeItem('ai-tutor:learning-home')
    setIntent(null)
    if (selected)
      void queryClient.invalidateQueries({
        queryKey: subjectQueryKeys.learningState(userId, selected.id),
      })
  }
  function openTraining(
    diagnosticId = learning.data?.diagnostic_session_id ?? '',
  ) {
    if (!selected || !diagnosticId) return
    const next: Intent = {
      apiBase,
      userId,
      subjectId: selected.id,
      sessionId: '',
      mode: 'training',
      diagnosticId,
    }
    sessionStorage.setItem('ai-tutor:learning-home', JSON.stringify(next))
    setIntent(next)
  }
  if (intent?.mode === 'training' && selected && userId)
    return (
      <LearningPlanEntry
        key={intent.diagnosticId}
        userId={userId}
        diagnosticId={
          intent.diagnosticId ?? learning.data?.diagnostic_session_id ?? ''
        }
        subjectName={selected.name}
        onBack={back}
      />
    )
  if (intent?.mode === 'diagnostic' && selected)
    return (
      <DiagnosticTrainer
        subjectId={selected.id}
        subjectName={selected.name}
        onBack={back}
        onTraining={(diagnosticId) => {
          void queryClient.invalidateQueries({
            queryKey: subjectQueryKeys.learningState(userId, selected.id),
          })
          openTraining(diagnosticId)
        }}
        initialSessionId={intent.sessionId || undefined}
        startNew={!intent.sessionId}
      />
    )

  const data = learning.data
  // Diagnostic and training are one process for the student: the course
  // offers a single action that resumes where they stopped.
  const stage = !data
    ? undefined
    : data.active_session_id
      ? 'diagnostic'
      : data.training_available && data.diagnostic_session_id
        ? 'training'
        : 'start'
  function resume() {
    if (!selected || !data) return
    if (stage === 'diagnostic')
      openDiagnostic(selected, data.active_session_id ?? undefined, false)
    else if (stage === 'training') openTraining()
    else openDiagnostic(selected, undefined, true)
  }
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <span className={styles.brandIcon} aria-hidden="true">
            <GraduationCap size={22} />
          </span>
          <Text weight="bold">{t('trainer.title')}</Text>
        </div>
        <AccountMenu />
      </header>
      <main className={styles.content}>
        <Heading as="h1" size="8" id="courses-title">
          {t('home.courses')}
        </Heading>
        {subjectsQuery.isPending ? (
          <Text role="status">{t('home.loading')}</Text>
        ) : subjectsQuery.isError ? (
          <Card className={styles.notice}>
            <Text role="alert">{t('home.loadError')}</Text>
            <Button onClick={() => void subjectsQuery.refetch()}>
              {t('trainer.retry')}
            </Button>
          </Card>
        ) : !selected ? (
          <Card className={styles.notice}>
            <Text>{t('home.empty')}</Text>
          </Card>
        ) : (
          // One course today; the list layout is ready for several.
          <section className={styles.courses} aria-labelledby="courses-title">
            <article className={styles.course} aria-labelledby="course-title">
              <div className={styles.courseHeader}>
                <Heading as="h2" size="7" id="course-title">
                  {selected.name}
                </Heading>
                <Text as="p" size="3" color="gray">
                  {t('home.intro')}
                </Text>
              </div>
              {/* Four coloured, numbered steps say what the student does: a voice
                  check, its gaps, a personal plan and exam preparation. They
                  are information, not controls. */}
              <ol className={styles.steps} aria-label={t('home.stepsLabel')}>
                {[
                  { key: 'check', icon: <Mic size={20} /> },
                  { key: 'gaps', icon: <Target size={20} /> },
                  { key: 'plan', icon: <Sparkles size={20} /> },
                  { key: 'exam', icon: <GraduationCap size={20} /> },
                ].map((step, index) => (
                  <li key={step.key} className={styles.step} data-step={index}>
                    <span className={styles.stepNumber} aria-hidden="true">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className={styles.stepIcon} aria-hidden="true">
                      {step.icon}
                    </span>
                    <Text as="p" className={styles.stepTitle}>
                      {t(`home.steps.${step.key}.title`)}
                    </Text>
                    <Text as="p" className={styles.stepText}>
                      {t(`home.steps.${step.key}.text`)}
                    </Text>
                  </li>
                ))}
              </ol>
              {learning.isPending ? (
                <Text role="status">{t('home.learningLoading')}</Text>
              ) : learning.isError ? (
                <Card className={styles.notice}>
                  <Text role="alert">{t('home.learningError')}</Text>
                  <Button onClick={() => void learning.refetch()}>
                    {t('trainer.retry')}
                  </Button>
                </Card>
              ) : data ? (
                <div className={styles.actions}>
                  {stage !== 'start' && (
                    <Text as="p" className={styles.status}>
                      {t(
                        stage === 'diagnostic'
                          ? 'home.statusDiagnostic'
                          : 'home.statusTraining',
                      )}
                    </Text>
                  )}
                  <div className={styles.actionRow}>
                    <CourseAction
                      kind="primary"
                      icon={<Zap size={22} fill="currentColor" />}
                      title={t(
                        stage === 'start'
                          ? 'home.startLearning'
                          : 'home.continueLearning',
                      )}
                      hint={t(
                        stage === 'start'
                          ? 'home.startLearningHint'
                          : stage === 'diagnostic'
                            ? 'home.continueDiagnosticHint'
                            : 'home.continueTrainingHint',
                      )}
                      onClick={resume}
                    />
                    {stage === 'training' && (
                      <CourseAction
                        kind="secondary"
                        icon={<RotateCcw size={22} />}
                        title={t('home.restart')}
                        hint={t('home.restartHint')}
                        onClick={() =>
                          openDiagnostic(selected, undefined, true)
                        }
                      />
                    )}
                  </div>
                </div>
              ) : null}
            </article>
          </section>
        )}
      </main>
    </div>
  )
}

// A large course action: icon, title and a line explaining what happens. The
// title is the accessible name; the explanation describes it.
function CourseAction({
  kind,
  icon,
  title,
  hint,
  onClick,
}: {
  kind: 'primary' | 'secondary'
  icon: ReactNode
  title: string
  hint: string
  onClick: () => void
}) {
  const hintId = useId()
  return (
    <button
      type="button"
      className={styles.courseAction}
      data-kind={kind}
      aria-label={title}
      aria-describedby={hintId}
      onClick={onClick}
    >
      <span className={styles.courseActionIcon} aria-hidden="true">
        {icon}
      </span>
      <span className={styles.courseActionText}>
        <span className={styles.courseActionTitle}>{title}</span>
        <span id={hintId} className={styles.courseActionHint}>
          {hint}
        </span>
      </span>
    </button>
  )
}
