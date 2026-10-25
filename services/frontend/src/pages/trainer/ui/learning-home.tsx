import { Badge, Button, Card, Heading, Text } from '@radix-ui/themes'
import { GraduationCap, Mic, Play, Sparkles, Target, Zap } from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { StudentCourseShell } from '@/widgets/student-course-shell'
import { AccountMenu, useAuth } from '@/features/auth'
import { useSubjectsQuery } from '@/features/subject-selection'
import {
  useDiagnosticHistoryQuery,
  useStartDiagnosticMutation,
} from '@/features/diagnostic-session'
import {
  DiagnosticApiError,
  type DiagnosticSummary,
} from '@/entities/diagnostic-session'
import { GeekDialog } from './geek-dialog'
import { GeekResult } from './geek-result'
import styles from './learning-home.module.scss'

// The open screen survives a reload of the tab.
type View = {
  userId: string
  subjectId: string
  sessionId: string
  screen: 'dialog' | 'result'
}
const viewKey = 'ai-tutor:geek-view'
const selectedSubjectKey = 'ai-tutor:selected-subject'

function readView(userId: string): View | null {
  try {
    const view = JSON.parse(sessionStorage.getItem(viewKey) ?? 'null') as View
    return view?.userId === userId &&
      view.sessionId &&
      (view.screen === 'dialog' || view.screen === 'result')
      ? view
      : null
  } catch {
    return null
  }
}

function saveView(view: View | null) {
  try {
    if (view) sessionStorage.setItem(viewKey, JSON.stringify(view))
    else sessionStorage.removeItem(viewKey)
  } catch {
    // Without storage the screen is simply not restored after a reload.
  }
}

function readSelectedSubject(userId: string) {
  try {
    return localStorage.getItem(`${selectedSubjectKey}:${userId}`) ?? ''
  } catch {
    return ''
  }
}

function saveSelectedSubject(userId: string, subjectId: string) {
  try {
    localStorage.setItem(`${selectedSubjectKey}:${userId}`, subjectId)
  } catch {
    // The selection still works for this page when storage is unavailable.
  }
}

export function LearningHome({
  requestedSubjectId,
}: {
  requestedSubjectId?: string
}) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const userId = user?.id ?? ''
  const subjectsQuery = useSubjectsQuery(userId)
  const subjects = subjectsQuery.data ?? []
  const [view, setView] = useState<View | null>(() =>
    userId ? readView(userId) : null,
  )
  const [savedSubjectId, setSavedSubjectId] = useState(() =>
    userId ? readSelectedSubject(userId) : '',
  )
  const selected = requestedSubjectId
    ? subjects.find((subject) => subject.id === requestedSubjectId)
    : (subjects.find((subject) => subject.id === view?.subjectId) ??
      subjects.find((subject) => subject.id === savedSubjectId) ??
      subjects[0])
  const history = useDiagnosticHistoryQuery(userId, selected?.id ?? '')
  const start = useStartDiagnosticMutation(userId, selected?.id ?? '')

  useEffect(() => {
    if (
      view &&
      !subjectsQuery.isPending &&
      subjects.length &&
      !subjects.some((s) => s.id === view.subjectId)
    ) {
      saveView(null)
      setView(null)
    }
  }, [view, subjects, subjectsQuery.isPending])

  useEffect(() => {
    if (selected) {
      setSavedSubjectId(selected.id)
      saveSelectedSubject(userId, selected.id)
    }
  }, [selected, userId])

  function open(sessionId: string, screen: View['screen']) {
    if (!selected) return
    const next = { userId, subjectId: selected.id, sessionId, screen }
    saveView(next)
    setView(next)
  }
  function back() {
    saveView(null)
    setView(null)
  }
  function begin() {
    start.mutate(undefined, {
      onSuccess: (session) => open(session.id, 'dialog'),
    })
  }

  const active = history.data?.find((item) => item.status === 'active')
  const startError =
    start.error instanceof DiagnosticApiError &&
    start.error.code === 'GRAPH_NOT_READY'
      ? t('home.graphNotReady')
      : start.error
        ? t('home.startError')
        : ''
  if (view && selected && view.subjectId === selected.id)
    return view.screen === 'dialog' ? (
      <GeekDialog
        key={view.sessionId}
        userId={userId}
        sessionId={view.sessionId}
        subjectName={selected.name}
        onBack={back}
        onResult={() => open(view.sessionId, 'result')}
      />
    ) : (
      <GeekResult
        key={view.sessionId}
        userId={userId}
        sessionId={view.sessionId}
        subjectName={selected.name}
        onBack={back}
        onAgain={begin}
        starting={start.isPending}
        startError={startError}
      />
    )

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
        <StudentCourseShell
          courseName={selected?.name}
          courses={subjects}
          selectedCourseId={selected?.id}
          onCourseChange={(subjectId) => {
            setSavedSubjectId(subjectId)
            saveSelectedSubject(userId, subjectId)
          }}
        />
        <Heading as="h1" size="8" id="courses-title">
          {t('home.title')}
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
          <section className={styles.courses} aria-labelledby="courses-title">
            <article className={styles.course} aria-labelledby="courses-title">
              <div className={styles.courseHeader}>
                <Text as="p" size="3" color="gray">
                  {t('home.intro')}
                </Text>
              </div>
              {/* Four numbered steps say what the student does; they are
                  information, not controls. */}
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
              <div className={styles.actions}>
                <div className={styles.actionRow}>
                  {active ? (
                    <CourseAction
                      icon={<Play size={22} fill="currentColor" />}
                      title={t('home.resume')}
                      hint={t('home.resumeHint')}
                      onClick={() => open(active.id, 'dialog')}
                    />
                  ) : (
                    <CourseAction
                      icon={<Zap size={22} fill="currentColor" />}
                      title={t(
                        start.isPending ? 'home.starting' : 'home.start',
                      )}
                      hint={t('home.startHint')}
                      disabled={start.isPending || history.isPending}
                      onClick={begin}
                    />
                  )}
                </div>
                {startError && (
                  <Text as="p" role="alert" color="red">
                    {startError}
                  </Text>
                )}
              </div>
            </article>
            <History
              query={history}
              onOpen={(item) =>
                open(item.id, item.status === 'completed' ? 'result' : 'dialog')
              }
            />
          </section>
        )}
      </main>
    </div>
  )
}

function History({
  query,
  onOpen,
}: {
  query: ReturnType<typeof useDiagnosticHistoryQuery>
  onOpen: (item: DiagnosticSummary) => void
}) {
  const { t, i18n } = useTranslation()
  return (
    <section className={styles.history} aria-labelledby="history-title">
      <Heading as="h2" size="5" id="history-title">
        {t('home.historyTitle')}
      </Heading>
      {query.isPending ? (
        <Text role="status">{t('home.historyLoading')}</Text>
      ) : query.isError ? (
        <Card className={styles.notice}>
          <Text role="alert">{t('home.historyError')}</Text>
          <Button onClick={() => void query.refetch()}>
            {t('trainer.retry')}
          </Button>
        </Card>
      ) : query.data.length === 0 ? (
        <Text as="p" color="gray">
          {t('home.historyEmpty')}
        </Text>
      ) : (
        <ul className={styles.historyList}>
          {query.data.map((item) => (
            <li key={item.id} className={styles.historyItem}>
              <span className={styles.historyText}>
                <Text weight="bold">
                  {t('home.historyItem', {
                    date: new Date(item.startedAt * 1000).toLocaleString(
                      i18n.language,
                      { dateStyle: 'medium', timeStyle: 'short' },
                    ),
                  })}
                </Text>
                <Text size="2" color="gray">
                  {t(`home.historyStatus.${item.status}`, {
                    progress: item.progressPercent,
                  })}
                </Text>
              </span>
              {item.status === 'completed' && (
                <Badge size="2" color={item.grade ? 'blue' : 'gray'}>
                  {item.grade && item.scorePercent !== null
                    ? t('home.historyGrade', {
                        grade: item.grade,
                        percent: item.scorePercent,
                      })
                    : t('home.historyNoGrade')}
                </Badge>
              )}
              {item.status !== 'cancelled' && (
                <Button variant="soft" onClick={() => onOpen(item)}>
                  {t(
                    item.status === 'completed'
                      ? 'home.openResult'
                      : 'home.continue',
                  )}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

// A large course action: icon, title and a line explaining what happens. The
// title is the accessible name; the explanation describes it.
function CourseAction({
  icon,
  title,
  hint,
  disabled,
  onClick,
}: {
  icon: ReactNode
  title: string
  hint: string
  disabled?: boolean
  onClick: () => void
}) {
  const hintId = useId()
  return (
    <button
      type="button"
      className={styles.courseAction}
      data-kind="primary"
      aria-label={title}
      aria-describedby={hintId}
      disabled={disabled}
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
