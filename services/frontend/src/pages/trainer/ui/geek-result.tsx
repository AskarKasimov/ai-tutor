import { Badge, Button, Heading, Text } from '@radix-ui/themes'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DiagnosticApiError,
  fetchDiagnosticMaterialRaw,
  type MaterialReference,
  type TopicStatus,
} from '@/entities/diagnostic-session'
import { useDiagnosticResultQuery } from '@/features/diagnostic-session'
import { TrainerShell } from './trainer-shared'
import styles from './geek.module.scss'

const statusTone: Record<TopicStatus, 'green' | 'blue' | 'red' | 'gray'> = {
  independent: 'green',
  assisted: 'blue',
  gap: 'red',
  insufficient: 'gray',
  credited_by_parent: 'green',
  unchecked: 'gray',
}

// The outcome of a completed diagnostic: the preliminary course grade, the
// gaps to work on and every topic with its status.
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
  return (
    <TrainerShell
      title={t('geek.resultTitle')}
      subject={subjectName}
      crumb={t('geek.resultTitle')}
      onBack={onBack}
    >
      <main className={styles.result}>
        <Heading as="h1" size="7">
          {t('geek.resultTitle')}
        </Heading>
        {query.isPending ? (
          <Text role="status">{t('geek.resultLoading')}</Text>
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
              className={styles.grade}
              data-grade={result.grade ?? 'none'}
              aria-labelledby="grade-title"
            >
              <Text as="p" id="grade-title" className={styles.gradeLabel}>
                {t('geek.gradeLabel')}
              </Text>
              <Text as="p" className={styles.gradeValue}>
                {result.grade ?? '—'}
              </Text>
              <Text as="p" color="gray">
                {result.scorePercent === null
                  ? t('geek.gradeNoneHint')
                  : t('geek.score', { percent: result.scorePercent })}
              </Text>
              <Text as="p" size="2" color="gray">
                {t('geek.gradeNote')}
              </Text>
            </section>
            <section aria-labelledby="gaps-title" className={styles.section}>
              <Heading as="h2" size="5" id="gaps-title">
                {t('geek.gapsTitle')}
              </Heading>
              {result.gaps.length === 0 ? (
                <Text as="p">{t('geek.gapsEmpty')}</Text>
              ) : (
                <>
                  <Text as="p" color="gray" size="2">
                    {t('geek.gapsHint')}
                  </Text>
                  <ul className={styles.gaps}>
                    {result.gaps.map((gap) => (
                      <li key={gap.nodeId}>
                        <Text weight="bold">{gap.title}</Text>
                        {gap.weeks.length > 0 && (
                          <Text as="p" size="2" color="gray">
                            {t('geek.weeks', { weeks: gap.weeks.join(', ') })}
                          </Text>
                        )}
                        {gap.sources.length === 0 ? (
                          <Text as="p" size="2" color="gray">
                            {t('geek.materialsNone')}
                          </Text>
                        ) : (
                          <div className={styles.gapMaterials}>
                            {(
                              ['theory', 'practice', 'additional'] as const
                            ).map((group) => {
                              const refs = gap.learningMaterials[group]
                              if (refs.length === 0) return null
                              return (
                                <section key={group}>
                                  <Text weight="medium">
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
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </section>
            <section aria-labelledby="topics-title" className={styles.section}>
              <Heading as="h2" size="5" id="topics-title">
                {t('geek.topicsTitle')}
              </Heading>
              <ul className={styles.topics}>
                {result.topics.map((topic) => (
                  <li key={topic.nodeId} className={styles.topic}>
                    <span className={styles.topicTitle}>
                      <Text weight={topic.topLevel ? 'bold' : 'regular'}>
                        {topic.title}
                      </Text>
                      {topic.topLevel && (
                        <Text size="1" color="gray">
                          {t('geek.topLevel')}
                        </Text>
                      )}
                    </span>
                    <Badge color={statusTone[topic.status]}>
                      {t(`geek.status.${topic.status}`)}
                    </Badge>
                    <Text className={styles.topicGrade}>
                      {topic.grade ?? '—'}
                    </Text>
                  </li>
                ))}
              </ul>
            </section>
          </>
        )}
        {startError && (
          <Text as="p" role="alert" color="red">
            {startError}
          </Text>
        )}
        <div className={styles.resultActions}>
          <Button size="3" disabled={starting} onClick={onAgain}>
            {t(starting ? 'home.starting' : 'geek.again')}
          </Button>
          <Button size="3" variant="soft" onClick={onBack}>
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
  return (
    <span>
      <Button
        size="1"
        variant="ghost"
        disabled={state === 'loading'}
        onClick={() => void open()}
      >
        {t('geek.openMaterial', { filename })}
      </Button>
      {material.kindLabel && (
        <Text as="span" size="1">
          {' '}
          · {material.kindLabel}
        </Text>
      )}
      {material.week && (
        <Text as="span" size="1">
          {' '}
          · {t('geek.materialWeek', { week: material.week })}
        </Text>
      )}
      {state === 'unavailable' && (
        <Text as="span" size="1" color="gray" role="status">
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
