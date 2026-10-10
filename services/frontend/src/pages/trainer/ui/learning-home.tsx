import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { ArrowRight, Dumbbell, GraduationCap, Lock, Target } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import {
  useLearningStateQuery,
  useSubjectsQuery,
} from '@/features/subject-selection'
import { subjectQueryKeys, type Subject } from '@/entities/subject'
import { TrainingPreview } from './training-preview'
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
  const [trainingRequested, setTrainingRequested] = useState(() => {
    const saved = userId ? readIntent(userId) : null
    return saved?.mode === 'training'
  })
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
    setTrainingRequested(false)
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
    setTrainingRequested(true)
  }
  if (intent?.mode === 'training' && selected && userId)
    return (
      <TrainingPreview
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
  const trainingOpen =
    !!data?.training_available && !!data.diagnostic_session_id
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
                <section className={styles.tiles}>
                  <article className={styles.tile} data-tone="diagnostic">
                    <span className={styles.tileIcon} aria-hidden="true">
                      <Target size={28} />
                    </span>
                    <Heading as="h3" size="6">
                      {t('home.diagnostic')}
                    </Heading>
                    <Text as="p" color="gray">
                      {t('home.diagnosticDescription')}
                    </Text>
                    {(data.diagnostic_completed ||
                      data.diagnostic_status === 'active') && (
                      <Text as="p" className={styles.status}>
                        {t(
                          data.diagnostic_completed
                            ? 'home.diagnosticComplete'
                            : 'home.diagnosticActive',
                        )}
                      </Text>
                    )}
                    <Button
                      size="3"
                      className={styles.tileAction}
                      onClick={() =>
                        data.active_session_id
                          ? openDiagnostic(
                              selected,
                              data.active_session_id,
                              false,
                            )
                          : openDiagnostic(selected, undefined, true)
                      }
                    >
                      {t(
                        data.active_session_id
                          ? 'home.continueDiagnostic'
                          : data.diagnostic_completed
                            ? 'home.retakeDiagnostic'
                            : 'home.startDiagnostic',
                      )}
                      <ArrowRight size={18} aria-hidden="true" />
                    </Button>
                  </article>
                  <article
                    className={styles.tile}
                    data-tone="training"
                    data-locked={!trainingOpen || undefined}
                  >
                    <span className={styles.tileIcon} aria-hidden="true">
                      {trainingOpen ? (
                        <Dumbbell size={28} />
                      ) : (
                        <Lock size={26} />
                      )}
                    </span>
                    <Heading as="h3" size="6">
                      {t('home.training')}
                    </Heading>
                    <Text as="p" color="gray">
                      {t('home.trainingDescription')}
                    </Text>
                    <Text as="p" className={styles.status}>
                      {data.training_available
                        ? t(
                            trainingRequested
                              ? 'home.trainingLoading'
                              : 'home.trainingReady',
                          )
                        : t('home.diagnosticRequired')}
                    </Text>
                    <Button
                      size="3"
                      className={styles.tileAction}
                      variant={trainingOpen ? 'solid' : 'soft'}
                      color={trainingOpen ? undefined : 'gray'}
                      disabled={!trainingOpen}
                      onClick={() => openTraining()}
                    >
                      {t(
                        trainingRequested
                          ? 'home.trainingLoading'
                          : data.training_available
                            ? 'home.openTraining'
                            : 'home.trainingLocked',
                      )}
                      {trainingOpen && (
                        <ArrowRight size={18} aria-hidden="true" />
                      )}
                    </Button>
                  </article>
                </section>
              ) : null}
            </article>
          </section>
        )}
      </main>
    </div>
  )
}
