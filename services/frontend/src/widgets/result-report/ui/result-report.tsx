import { Button, Heading, Text } from '@radix-ui/themes'
import { CheckCircle2, AlertTriangle, ChevronRight } from 'lucide-react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkdownContent } from '@/shared/ui'
import type {
  ReportMark,
  ReportStatus,
  ReportSummary,
  ReportTopic,
  ResultReportData,
} from '../model/report'
import { ResultGraph } from './result-graph'
import styles from './result-report.module.scss'

const statusOrder: ReportStatus[] = [
  'full',
  'partial',
  'gap',
  'credited',
  'insufficient',
]

// The one result screen of the learning diagnostic and the exam: the overall
// grade, the conclusion, what went well and what to work on, the topics in
// points and their graph.
export function ResultReport({
  report,
  summary,
  actions,
  topicExtra,
  children,
}: {
  report: ResultReportData
  summary: ReportSummary
  actions?: ReactNode
  // What the mode can add to a topic to work on, such as its materials.
  topicExtra?: (topic: ReportTopic) => ReactNode
  // Below the graph: what only this mode has.
  children?: ReactNode
}) {
  const { t } = useTranslation()
  const checked = report.topics.filter((topic) => topic.status !== 'unchecked')
  const counts = statusOrder
    .map((status) => ({
      status,
      count: checked.filter((topic) => topic.status === status).length,
    }))
    .filter(({ count }) => count)
  const good = checked.filter((topic) => topic.status === 'full')
  const work = [
    ...checked.filter((topic) => topic.status === 'gap'),
    ...checked.filter((topic) => topic.status === 'partial'),
  ]
  return (
    <div className={styles.result}>
      <Heading as="h1" size="7" className={styles.title}>
        {t('resultReport.title')}
      </Heading>
      <section
        className={styles.hero}
        data-grade={report.grade ?? 'none'}
        aria-labelledby="grade-title"
      >
        {/* The ring fills with the share of the maximum score. */}
        <div
          className={styles.ring}
          style={{ '--score': report.percent ?? 0 } as CSSProperties}
        >
          <Text as="p" className={styles.gradeValue}>
            {report.grade ?? '—'}
          </Text>
        </div>
        <div className={styles.gradeCopy}>
          <Text as="p" id="grade-title" className={styles.eyebrow}>
            {t('resultReport.gradeLabel')}
          </Text>
          <Text as="p" className={styles.score}>
            {report.grade
              ? t(`resultReport.grade.${report.grade}`)
              : t('resultReport.gradeNone')}
          </Text>
          {report.grade && (
            <Text as="p" className={styles.points}>
              {t('resultReport.totalPoints', {
                score: report.score,
                max: report.maxScore,
                count: report.maxScore,
              })}
            </Text>
          )}
          <Text as="p" className={styles.note}>
            {t('resultReport.gradeNote')}
          </Text>
        </div>
        {counts.length > 0 && (
          <ul
            className={styles.stats}
            aria-label={t('resultReport.statusSummary')}
          >
            {counts.map(({ status, count }) => (
              <li key={status} data-status={status}>
                <span className={styles.statValue}>{count}</span>
                <span className={styles.statLabel}>
                  {t(`resultReport.status.${status}`)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
      <Summary summary={summary} />
      {(good.length > 0 || work.length > 0) && (
        <div className={styles.cards}>
          {good.length > 0 && (
            <section
              className={styles.card}
              data-kind="good"
              aria-labelledby="good-title"
            >
              <Heading
                as="h2"
                size="4"
                id="good-title"
                className={styles.cardTitle}
              >
                <CheckCircle2 size={18} aria-hidden="true" />
                {t('resultReport.goodTitle')}
              </Heading>
              <FindingList topics={good} />
            </section>
          )}
          {work.length > 0 && (
            <section
              className={styles.card}
              data-kind="work"
              aria-labelledby="work-title"
            >
              <Heading
                as="h2"
                size="4"
                id="work-title"
                className={styles.cardTitle}
              >
                <AlertTriangle size={18} aria-hidden="true" />
                {t('resultReport.workTitle')}
              </Heading>
              <FindingList topics={work} extra={topicExtra} />
            </section>
          )}
        </div>
      )}
      <section aria-labelledby="topics-title" className={styles.panel}>
        <Heading as="h2" size="5" id="topics-title">
          {t('resultReport.topicsTitle')}
        </Heading>
        {checked.length === 0 ? (
          <Text as="p" className={styles.lead}>
            {t('resultReport.topicsEmpty')}
          </Text>
        ) : (
          <ul className={styles.topics}>
            {checked.map((topic) => (
              <TopicRow key={topic.id} topic={topic} />
            ))}
          </ul>
        )}
      </section>
      <ResultGraph topics={report.topics} />
      {children}
      {actions && <div className={styles.actions}>{actions}</div>}
    </div>
  )
}

function Summary({ summary }: { summary: ReportSummary }) {
  const { t } = useTranslation()
  if (summary.state === 'ready' && !summary.text) return null
  return (
    <section
      className={styles.summary}
      aria-label={t('resultReport.summaryTitle')}
    >
      {summary.state === 'loading' && (
        <div role="status" className={styles.summaryLoading}>
          <Text as="p">{t('resultReport.summaryLoading')}</Text>
          <div className={styles.skeleton} aria-hidden="true">
            <span />
            <span />
          </div>
        </div>
      )}
      {summary.state === 'error' && (
        <div className={styles.error} role="alert">
          <Text as="p">{t('resultReport.summaryError')}</Text>
          <Button
            variant="soft"
            disabled={summary.retrying}
            onClick={summary.onRetry}
          >
            {t('resultReport.summaryRetry')}
          </Button>
        </div>
      )}
      {summary.state === 'ready' && <MarkdownContent text={summary.text} />}
    </section>
  )
}

function FindingList({
  topics,
  extra,
}: {
  topics: ReportTopic[]
  extra?: (topic: ReportTopic) => ReactNode
}) {
  return (
    <ul className={styles.findings}>
      {topics.map((topic) => (
        <li key={topic.id}>
          <strong className={styles.findingTitle}>{topic.title}</strong>
          {topic.note && <MarkdownContent text={topic.note} />}
          {extra?.(topic)}
        </li>
      ))}
    </ul>
  )
}

function TopicRow({ topic }: { topic: ReportTopic }) {
  const { t } = useTranslation()
  const line = (
    <>
      <span className={styles.topicTitle}>
        <Text className={styles.topicName}>{topic.title}</Text>
        <span className={styles.topicMeta}>
          <span className={styles.status}>
            {t(`resultReport.status.${topic.status}`)}
          </span>
          {topic.skipped && (
            <Text className={styles.topLevel}>{t('resultReport.skipped')}</Text>
          )}
          {topic.assisted && (
            <Text className={styles.topLevel}>
              {t('resultReport.assisted')}
            </Text>
          )}
        </span>
      </span>
      {topic.score !== null && (
        <span className={styles.topicPoints}>
          {t('resultReport.points', {
            score: topic.score,
            max: topic.maxScore,
          })}
        </span>
      )}
    </>
  )
  return (
    <li
      className={styles.topic}
      data-status={topic.status}
      data-top={topic.main || undefined}
    >
      {topic.marks.length === 0 ? (
        <div className={styles.topicLine}>{line}</div>
      ) : (
        <details className={styles.topicDetails}>
          <summary className={styles.topicLine}>
            <ChevronRight
              size={16}
              className={styles.topicChevron}
              aria-hidden="true"
            />
            {line}
          </summary>
          <ul className={styles.marks}>
            {topic.marks.map((mark, index) => (
              <MarkRow key={mark.id} mark={mark} index={index} />
            ))}
          </ul>
        </details>
      )}
    </li>
  )
}

function MarkRow({ mark, index }: { mark: ReportMark; index: number }) {
  const { t } = useTranslation()
  const title =
    mark.title ||
    (/^k[1-4]$/.test(mark.id)
      ? t(`resultReport.criteria.${mark.id}`)
      : t('resultReport.markFallback', { number: index + 1 }))
  return (
    <li className={styles.mark}>
      <span className={styles.markPoints}>
        {t('resultReport.points', { score: mark.score, max: mark.maxScore })}
      </span>
      <span className={styles.markBody}>
        <Text className={styles.markTitle}>{title}</Text>
        {mark.reason && (
          <Text className={styles.markReason}>{mark.reason}</Text>
        )}
        {mark.quote && <Text className={styles.markQuote}>«{mark.quote}»</Text>}
      </span>
    </li>
  )
}
