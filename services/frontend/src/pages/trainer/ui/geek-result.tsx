import { Button, Heading, Text } from '@radix-ui/themes'
import { FileText } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DiagnosticApiError,
  type DiagnosticGap,
  fetchDiagnosticMaterialRaw,
  type MaterialReference,
} from '@/entities/diagnostic-session'
import {
  useDiagnosticResultQuery,
  useDiagnosticResultSummaryQuery,
} from '@/features/diagnostic-session'
import { ResultReport, reportSummary } from '@/widgets/result-report'
import { toResultReport } from '../lib/to-result-report'
import { TrainerShell } from './trainer-shared'
import styles from './geek-result.module.scss'

// The outcome of a completed diagnostic on the result screen shared with the
// learning diagnostic; what only the exam has are the materials of a topic.
export function GeekResult({
  userId,
  sessionId,
  subjectName,
  onBack,
  onAgain,
  onChat,
  starting = false,
  startError = '',
}: {
  userId: string
  sessionId: string
  subjectName: string
  onBack: () => void
  onAgain: () => void
  onChat: () => void
  // The state of "start again", which runs from this screen.
  starting?: boolean
  startError?: string
}) {
  const { t } = useTranslation()
  const query = useDiagnosticResultQuery(userId, sessionId)
  const summaryQuery = useDiagnosticResultSummaryQuery(userId, sessionId)
  const result = query.data
  const report = useMemo(() => result && toResultReport(result), [result])
  const gaps = useMemo(
    () =>
      new Map(
        [...(result?.gaps ?? []), ...(result?.weakSpots ?? [])].map((gap) => [
          gap.nodeId,
          gap,
        ]),
      ),
    [result],
  )
  const summary = reportSummary(summaryQuery, (text) => text)
  const actions = (
    <>
      <Button size="3" disabled={starting} onClick={onAgain}>
        {t(starting ? 'home.starting' : 'geek.again')}
      </Button>
      <Button size="3" variant="soft" onClick={onChat}>
        {t('geek.openChat')}
      </Button>
      <Button size="3" variant="soft" color="gray" onClick={onBack}>
        {t('home.back')}
      </Button>
    </>
  )
  return (
    <TrainerShell
      title={t('geek.resultTitle')}
      subject={subjectName}
      crumb={t('geek.resultTitle')}
      onBack={onBack}
    >
      <main>
        {query.isPending ? (
          <Text role="status" className={styles.muted}>
            {t('geek.resultLoading')}
          </Text>
        ) : query.isError || !report ? (
          <div role="alert">
            <Text as="p">{t('geek.resultError')}</Text>
            <Button variant="soft" onClick={() => void query.refetch()}>
              {t('trainer.retry')}
            </Button>
          </div>
        ) : (
          <ResultReport
            report={report}
            summary={summary}
            actions={actions}
            topicExtra={(topic) => {
              const gap = gaps.get(topic.id)
              return gap ? (
                <GapMaterials gap={gap} sessionId={sessionId} />
              ) : null
            }}
          />
        )}
        {startError && (
          <Text as="p" role="alert" color="red">
            {startError}
          </Text>
        )}
      </main>
    </TrainerShell>
  )
}

// The weeks of a topic to work on and the materials to learn it from.
function GapMaterials({
  gap,
  sessionId,
}: {
  gap: DiagnosticGap
  sessionId: string
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.extra}>
      {gap.weeks.length > 0 && (
        <Text as="p" className={styles.chip}>
          {t('geek.weeks', { weeks: gap.weeks.join(', ') })}
        </Text>
      )}
      {gap.sources.length === 0 ? (
        <Text as="p" className={styles.muted}>
          {t('geek.materialsNone')}
        </Text>
      ) : (
        <div className={styles.materials}>
          {(['theory', 'practice', 'additional'] as const).map((group) => {
            const refs = gap.learningMaterials[group]
            if (refs.length === 0) return null
            return (
              <section key={group} className={styles.materialGroup}>
                <Heading as="h3" size="1" className={styles.groupLabel}>
                  {t(`geek.${group}`)}
                </Heading>
                <ul>
                  {refs.map((ref) => (
                    <li key={ref.materialId}>
                      <MaterialDownload sessionId={sessionId} material={ref} />
                    </li>
                  ))}
                </ul>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}

function MaterialDownload({
  sessionId,
  material,
}: {
  sessionId: string
  material: MaterialReference
}) {
  const { t } = useTranslation()
  const [state, setState] = useState<
    'idle' | 'loading' | 'unavailable' | 'error'
  >('idle')
  const filename =
    material.filename || material.kindLabel || material.materialId
  async function open() {
    setState('loading')
    try {
      const blob = await fetchDiagnosticMaterialRaw(
        sessionId,
        material.materialId,
        new AbortController().signal,
      )
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = filename
      document.body.append(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
      setState('idle')
    } catch (error) {
      setState(
        error instanceof DiagnosticApiError && error.status === 410
          ? 'unavailable'
          : 'error',
      )
    }
  }
  const meta = [
    material.kindLabel,
    material.week && t('geek.materialWeek', { week: material.week }),
  ].filter(Boolean)
  return (
    <span className={styles.material}>
      <button
        type="button"
        className={styles.materialButton}
        disabled={state === 'loading'}
        aria-label={t('geek.openMaterial', { filename })}
        onClick={() => void open()}
      >
        <FileText size={16} aria-hidden="true" />
        <span className={styles.materialName}>{filename}</span>
      </button>
      {meta.length > 0 && (
        <Text as="span" className={styles.materialMeta}>
          {meta.join(' · ')}
        </Text>
      )}
      {state === 'unavailable' && (
        <Text as="span" className={styles.materialMeta} role="status">
          {t('geek.materialUnavailable')}
        </Text>
      )}
      {state === 'error' && (
        <Text as="span" size="1" color="red" role="alert">
          {t('geek.materialDownloadError')}
        </Text>
      )}
    </span>
  )
}
