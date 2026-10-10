import { ArrowRight, History, Sparkles } from 'lucide-react'
import { Button, Card, Heading, Text } from '@radix-ui/themes'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  TrainingApiError,
  type TrainingPreview,
  type TrainingProgress,
  type TrainingTarget,
} from '@/entities/training'
import {
  useTrainingEntryQuery,
  useStartTrainingMutation,
} from '@/features/training'
import { TrainingSession } from './training-session'
import { TrainerShell } from './trainer-shared'
import styles from './trainer-layout.module.scss'
import previewStyles from './training-preview.module.scss'

function TargetList({
  targets,
  empty,
}: {
  targets: TrainingTarget[]
  empty: string
}) {
  const { t } = useTranslation()
  return targets.length ? (
    <ul>
      {targets.map((target, index) => (
        <li key={`${target.outcome_id}-${index}`}>
          <Text>
            {target.competency_name} · {target.outcome_name}
          </Text>
          <Text as="p">
            {target.original_score === null
              ? t('training.untested')
              : t('training.originalScore', {
                  score: target.original_score,
                  max: target.original_max_score,
                })}
          </Text>
        </li>
      ))}
    </ul>
  ) : (
    <Text>{empty}</Text>
  )
}

export function TrainingPreview({
  userId,
  diagnosticId,
  subjectName,
  onBack,
}: {
  userId: string
  diagnosticId: string
  subjectName: string
  onBack: () => void
}) {
  const { t } = useTranslation()
  const entry = useTrainingEntryQuery(userId, diagnosticId)
  const start = useStartTrainingMutation(userId, diagnosticId)
  const [progress, setProgress] = useState<TrainingProgress>()
  const preview =
    entry.data?.kind === 'preview' ? entry.data.preview : undefined
  if (progress)
    return (
      <TrainingSession
        key={progress.session_id}
        userId={userId}
        initial={progress}
        onBack={onBack}
        onMissing={() => setProgress(undefined)}
      />
    )
  if (
    entry.isError &&
    entry.error instanceof TrainingApiError &&
    entry.error.status === 404
  )
    return (
      <TrainerShell title={t('training.title')} subject={subjectName}>
        <main className={styles.preview}>
          <Heading as="h1">{t('training.title')}</Heading>
          <Text role="alert">{t('training.sessionMissing')}</Text>
          <Button onClick={onBack}>{t('home.back')}</Button>
        </main>
      </TrainerShell>
    )
  return (
    <TrainerShell
      title={t('training.title')}
      subject={subjectName}
      onBack={onBack}
    >
      <main className={styles.preview}>
        <TrainingHero free={preview?.mode === 'free_practice'} />
        {entry.isPending ? (
          <Text role="status">{t('training.previewLoading')}</Text>
        ) : entry.isError ? (
          <Card>
            <Text role="alert">
              {entry.error instanceof TrainingApiError &&
              entry.error.status === 401
                ? t('trainer.unauthorized')
                : t('training.previewError')}
            </Text>
            <Button onClick={() => void entry.refetch()}>
              {t('trainer.retry')}
            </Button>
          </Card>
        ) : entry.data?.kind === 'existing' ? (
          // An unfinished training only needs a clear way back in.
          <section
            className={previewStyles.resume}
            aria-labelledby="resume-title"
          >
            <span className={previewStyles.resumeIcon} aria-hidden="true">
              <History size={24} />
            </span>
            <div className={previewStyles.resumeText}>
              <Heading as="h2" id="resume-title" size="5">
                {t('training.resumeTitle', {
                  round: entry.data.progress.round,
                })}
              </Heading>
              <Text as="p">
                {entry.data.progress.answer_count > 0
                  ? t('training.resumeHelp', {
                      count: entry.data.progress.answer_count,
                    })
                  : t('training.resumeFirst')}
              </Text>
            </div>
            <Button
              size="4"
              className={previewStyles.resumeAction}
              onClick={() => {
                if (entry.data?.kind === 'existing')
                  setProgress(entry.data.progress)
              }}
            >
              {t('training.continue')}
              <ArrowRight size={20} aria-hidden="true" />
            </Button>
          </section>
        ) : preview ? (
          <PreviewContent
            preview={preview}
            start={start}
            onStarted={setProgress}
          />
        ) : null}
      </main>
    </TrainerShell>
  )
}
// Explains what the training is and why it pays off before the plan.
function TrainingHero({ free }: { free: boolean }) {
  const { t } = useTranslation()
  return (
    <header className={previewStyles.hero}>
      <Heading as="h1">{t('training.title')}</Heading>
      <Text as="p" className={previewStyles.lead}>
        {t(free ? 'training.leadFree' : 'training.lead')}
      </Text>
      <Text as="p" className={previewStyles.motivation}>
        <Sparkles size={20} aria-hidden="true" />
        {t(free ? 'training.motivationFree' : 'training.motivation')}
      </Text>
    </header>
  )
}

function PreviewContent({
  preview,
  start,
  onStarted,
}: {
  preview: TrainingPreview
  start: ReturnType<typeof useStartTrainingMutation>
  onStarted: (progress: TrainingProgress) => void
}) {
  const { t } = useTranslation()
  const noTasks = preview.status === 'no_practice_tasks'
  const stale =
    start.error instanceof TrainingApiError && start.error.status === 409
  const error = start.error
  return (
    <>
      <Card>
        <Heading as="h2">
          {preview.mode === 'free_practice'
            ? t('training.freePractice')
            : t('training.focusedTitle')}
        </Heading>
        <Text>
          {t('training.diagnosticScore', {
            score: preview.diagnostic_score,
            max: preview.maximum_score,
          })}
        </Text>
        {preview.mode === 'focused' ? (
          <>
            <Heading as="h3">{t('training.confirmedGaps')}</Heading>
            <TargetList
              targets={preview.confirmed_gaps}
              empty={t('training.noConfirmedGaps')}
            />
            <Heading as="h3">{t('training.partialGaps')}</Heading>
            <TargetList
              targets={preview.partial_competencies}
              empty={t('training.noPartialGaps')}
            />
          </>
        ) : (
          <TargetList
            targets={preview.topics}
            empty={t('training.noTargets')}
          />
        )}
        {stale && <Text role="status">{t('training.stalePreview')}</Text>}
        {noTasks && <Text role="status">{t('training.noPracticeTasks')}</Text>}
        {error && !stale && (
          <Text role="alert">
            {error instanceof TrainingApiError && error.status === 401
              ? t('trainer.unauthorized')
              : t('training.startError')}
          </Text>
        )}
        <Button
          disabled={noTasks || start.isPending}
          onClick={() =>
            start.mutate(
              { preview },
              { onSuccess: (result) => onStarted(result.progress) },
            )
          }
        >
          {t(start.isPending ? 'training.starting' : 'training.start')}
        </Button>
      </Card>
    </>
  )
}
