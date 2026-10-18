import { Button, Heading, Text } from '@radix-ui/themes'
import { FileText } from 'lucide-react'
import { type CSSProperties, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DiagnosticApiError,
  fetchDiagnosticMaterialRaw,
  type MaterialReference,
  type TopicStatus,
} from '@/entities/diagnostic-session'
import { useDiagnosticResultQuery } from '@/features/diagnostic-session'
import { ResultGraph } from './result-graph'
import { TrainerShell } from './trainer-shared'
import styles from './geek-result.module.scss'

// The order of the status summary: from mastered to unclear. Topics the
// diagnostic never reached (unchecked) are not part of the feedback.
const statusOrder: TopicStatus[] = [
  'independent',
  'credited_by_parent',
  'assisted',
  'gap',
  'insufficient',
]

// The outcome of a completed diagnostic: the preliminary course grade, the
// gaps to work on and every checked topic with its status.
export function GeekResult({
  userId,
  sessionId,
  subjectName,
  onBack,
  onAgain,
  starting = false,
  startError = '',
}: {
  userId: string
  sessionId: string
  subjectName: string
  onBack: () => void
  onAgain: () => void
  // The state of "start again", which runs from this screen.
  starting?: boolean
  startError?: string
}) {
  const { t } = useTranslation()
  const query = useDiagnosticResultQuery(userId, sessionId)
  const result = query.data
  const checked =
    result?.topics.filter((topic) => topic.status !== 'unchecked') ?? []
  const counts = statusOrder
    .map((status) => ({
      status,
      count: checked.filter((topic) => topic.status === status).length,
    }))
    .filter(({ count }) => count)
  return (
    <TrainerShell
      title={t('geek.resultTitle')}
      subject={subjectName}
      crumb={t('geek.resultTitle')}
      onBack={onBack}
    >
      <main className={styles.result}>
        <header className={styles.intro}>
          <Text as="p" className={styles.eyebrow}>
            {subjectName}
          </Text>
          <Heading as="h1" size="7" className={styles.title}>
            {t('geek.resultTitle')}
          </Heading>
        </header>
        {query.isPending ? (
          <Text role="status" className={styles.muted}>
            {t('geek.resultLoading')}
          </Text>
        ) : query.isError || !result ? (
          <div className={styles.error} role="alert">
            <Text as="p">{t('geek.resultError')}</Text>
            <Button variant="soft" onClick={() => void query.refetch()}>
              {t('trainer.retry')}
            </Button>
          </div>
        ) : (
          <>
            <section
              className={styles.hero}
              data-grade={result.grade ?? 'none'}
              aria-labelledby="grade-title"
            >
              {/* The ring fills with the share of the maximum score. */}
              <div
                className={styles.ring}
                style={{ '--score': result.scorePercent ?? 0 } as CSSProperties}
              >
                <Text as="p" className={styles.gradeValue}>
                  {result.grade ?? '—'}
                </Text>
              </div>
              <div className={styles.gradeCopy}>
                <Text as="p" id="grade-title" className={styles.eyebrow}>
                  {t('geek.gradeLabel')}
                </Text>
                <Text as="p" className={styles.score}>
                  {result.scorePercent === null
                    ? t('geek.gradeNoneHint')
                    : t('geek.score', { percent: result.scorePercent })}
                </Text>
                <Text as="p" className={styles.note}>
                  {t('geek.gradeNote')}
                </Text>
              </div>
              {counts.length > 0 && (
                <ul
                  className={styles.stats}
                  aria-label={t('geek.statusSummary')}
                >
                  {counts.map(({ status, count }) => (
                    <li key={status} data-status={status}>
                      <span className={styles.statValue}>{count}</span>
                      <span className={styles.statLabel}>
                        {t(`geek.status.${status}`)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </section>
            <div className={styles.columns}>
              <section aria-labelledby="gaps-title" className={styles.panel}>
                <Heading as="h2" size="5" id="gaps-title">
                  {t('geek.gapsTitle')}
                </Heading>
                {result.gaps.length === 0 ? (
                  <Text as="p" className={styles.lead}>
                    {t('geek.gapsEmpty')}
                  </Text>
                ) : (
                  <>
                    <Text as="p" className={styles.lead}>
                      {t('geek.gapsHint')}
                    </Text>
                    <ol className={styles.gaps}>
                      {result.gaps.map((gap, index) => (
                        <li key={gap.nodeId} className={styles.gap}>
                          <span className={styles.gapIndex} aria-hidden="true">
                            {String(index + 1).padStart(2, '0')}
                          </span>
                          <div className={styles.gapBody}>
                            <Heading
                              as="h3"
                              size="4"
                              className={styles.gapTitle}
                            >
                              {gap.title}
                            </Heading>
                            {gap.weeks.length > 0 && (
                              <Text as="p" className={styles.chip}>
                                {t('geek.weeks', {
                                  weeks: gap.weeks.join(', '),
                                })}
                              </Text>
                            )}
                            {gap.sources.length === 0 ? (
                              <Text as="p" className={styles.muted}>
                                {t('geek.materialsNone')}
                              </Text>
                            ) : (
                              <div className={styles.materials}>
                                {(
                                  ['theory', 'practice', 'additional'] as const
                                ).map((group) => {
                                  const refs = gap.learningMaterials[group]
                                  if (refs.length === 0) return null
                                  return (
                                    <section
                                      key={group}
                                      className={styles.materialGroup}
                                    >
                                      <Text
                                        as="p"
                                        className={styles.groupLabel}
                                      >
                                        {t(`geek.${group}`)}
                                      </Text>
                                      <ul>
                                        {refs.map((ref) => (
                                          <li key={ref.materialId}>
                                            <MaterialDownload
                                              sessionId={sessionId}
                                              material={ref}
                                            />
                                          </li>
                                        ))}
                                      </ul>
                                    </section>
                                  )
                                })}
                              </div>
                            )}
                          </div>
                        </li>
                      ))}
                    </ol>
                  </>
                )}
              </section>
              <section aria-labelledby="topics-title" className={styles.panel}>
                <Heading as="h2" size="5" id="topics-title">
                  {t('geek.topicsTitle')}
                </Heading>
                {checked.length === 0 ? (
                  <Text as="p" className={styles.lead}>
                    {t('geek.topicsEmpty')}
                  </Text>
                ) : (
                  <ul className={styles.topics}>
                    {checked.map((topic) => (
                      <li
                        key={topic.nodeId}
                        className={styles.topic}
                        data-status={topic.status}
                        data-top={topic.topLevel || undefined}
                      >
                        <span className={styles.topicTitle}>
                          <Text className={styles.topicName}>
                            {topic.title}
                          </Text>
                          <span className={styles.topicMeta}>
                            <span className={styles.status}>
                              {t(`geek.status.${topic.status}`)}
                            </span>
                            {topic.topLevel && (
                              <Text className={styles.topLevel}>
                                {t('geek.topLevel')}
                              </Text>
                            )}
                          </span>
                        </span>
                        <span className={styles.topicGrade}>
                          {topic.grade ?? '—'}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            </div>
            <ResultGraph topics={result.topics} />
          </>
        )}
        {startError && (
          <Text as="p" role="alert" color="red">
            {startError}
          </Text>
        )}
        <div className={styles.actions}>
          <Button
            size="3"
            className={styles.primary}
            disabled={starting}
            onClick={onAgain}
          >
            {t(starting ? 'home.starting' : 'geek.again')}
          </Button>
          <Button size="3" variant="soft" color="gray" onClick={onBack}>
            {t('home.back')}
          </Button>
        </div>
      </main>
    </TrainerShell>
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
