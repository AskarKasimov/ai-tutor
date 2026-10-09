import { Button, Card, Heading, Select, Text } from '@radix-ui/themes'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/features/auth'
import {
  useLearningStateQuery,
  useSubjectsQuery,
} from '@/features/subject-selection'
import type { Subject } from '@/entities/subject'
import { DiagnosticTrainer } from './diagnostic-trainer'
import { AccountMenu } from '@/features/auth'
import styles from './learning-home.module.scss'

type Intent = {
  apiBase: string
  userId: string
  subjectId: string
  sessionId: string
  mode: 'diagnostic'
}
const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
function readIntent(userId: string): Intent | null {
  try {
    const value = JSON.parse(
      sessionStorage.getItem('ai-tutor:learning-home') ?? 'null',
    ) as Intent | null
    return value?.userId === userId &&
      value.apiBase === apiBase &&
      value.mode === 'diagnostic'
      ? value
      : null
  } catch {
    return null
  }
}

export function LearningHome() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const userId = user?.id ?? ''
  const subjectsQuery = useSubjectsQuery(userId)
  const [selectedId, setSelectedId] = useState('')
  const [trainingRequested, setTrainingRequested] = useState(false)
  const [intent, setIntent] = useState<Intent | null>(() =>
    userId ? readIntent(userId) : null,
  )
  const subjects = subjectsQuery.data ?? []
  const selected =
    subjects.find((subject) => subject.id === selectedId) ??
    subjects.find((subject) => subject.id === intent?.subjectId)
  const learning = useLearningStateQuery(userId, selected?.id ?? '')

  useEffect(() => setTrainingRequested(false), [selectedId])

  useEffect(() => {
    if (
      intent?.subjectId &&
      subjects.some((s) => s.id === intent.subjectId) &&
      !selectedId
    )
      setSelectedId(intent.subjectId)
    if (
      intent &&
      !subjectsQuery.isPending &&
      subjects.length &&
      !subjects.some((s) => s.id === intent.subjectId)
    ) {
      sessionStorage.removeItem('ai-tutor:learning-home')
      setIntent(null)
    }
  }, [intent, selectedId, subjects, subjectsQuery.isPending])

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
  }
  if (intent && selected)
    return (
      <DiagnosticTrainer
        subjectId={selected.id}
        subjectName={selected.name}
        onBack={back}
        initialSessionId={intent.sessionId || undefined}
        startNew={!intent.sessionId}
      />
    )

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <div>
          <Text>{t('trainer.title')}</Text>
          <Heading as="h1">{t('home.title')}</Heading>
        </div>
        <AccountMenu />
      </header>
      {subjectsQuery.isPending ? (
        <Text role="status">{t('home.loading')}</Text>
      ) : subjectsQuery.isError ? (
        <Card>
          <Text role="alert">{t('home.loadError')}</Text>
          <Button onClick={() => void subjectsQuery.refetch()}>
            {t('trainer.retry')}
          </Button>
        </Card>
      ) : subjects.length === 0 ? (
        <Card>
          <Text>{t('home.empty')}</Text>
        </Card>
      ) : (
        <>
          <label className={styles.selector}>
            <Text>{t('home.subject')}</Text>
            <Select.Root
              value={selected?.id ?? ''}
              onValueChange={setSelectedId}
            >
              <Select.Trigger aria-label={t('home.subject')} />
              <Select.Content>
                {subjects.map((subject) => (
                  <Select.Item key={subject.id} value={subject.id}>
                    {subject.name}
                  </Select.Item>
                ))}
              </Select.Content>
            </Select.Root>
          </label>
          {selected && (
            <section className={styles.actions}>
              {learning.isPending ? (
                <Text role="status">{t('home.learningLoading')}</Text>
              ) : learning.isError ? (
                <Card>
                  <Text role="alert">{t('home.learningError')}</Text>
                  <Button onClick={() => void learning.refetch()}>
                    {t('trainer.retry')}
                  </Button>
                </Card>
              ) : learning.data ? (
                <>
                  <Card>
                    <Heading as="h2">{t('home.diagnostic')}</Heading>
                    <Text>
                      {learning.data.diagnostic_completed
                        ? t('home.diagnosticComplete')
                        : learning.data.diagnostic_status === 'active'
                          ? t('home.diagnosticActive')
                          : t('home.diagnosticNeeded')}
                    </Text>
                    {learning.data.diagnostic_status === 'active' &&
                    learning.data.active_session_id ? (
                      <Button
                        onClick={() =>
                          openDiagnostic(
                            selected,
                            learning.data.active_session_id,
                            false,
                          )
                        }
                      >
                        {t('home.continueDiagnostic')}
                      </Button>
                    ) : (
                      <Button
                        onClick={() =>
                          openDiagnostic(selected, undefined, true)
                        }
                      >
                        {t('home.startDiagnostic')}
                      </Button>
                    )}
                  </Card>
                  <Card>
                    <Heading as="h2">
                      {learning.data.diagnostic_completed
                        ? t('home.training')
                        : t('home.trainingLocked')}
                    </Heading>
                    <Text>
                      {learning.data.training_available
                        ? t(
                            trainingRequested
                              ? 'home.trainingLoading'
                              : 'home.trainingReady',
                          )
                        : t('home.diagnosticRequired')}
                    </Text>
                    <Button
                      disabled={!learning.data.training_available}
                      onClick={() => setTrainingRequested(true)}
                    >
                      {t(
                        learning.data.training_available
                          ? 'home.openTraining'
                          : 'home.trainingLocked',
                      )}
                    </Button>
                  </Card>
                </>
              ) : null}
            </section>
          )}
        </>
      )}
    </main>
  )
}
