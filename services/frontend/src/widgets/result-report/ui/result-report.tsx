import { Button, Heading, Text } from '@radix-ui/themes'
import { CheckCircle2, AlertTriangle } from 'lucide-react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkdownContent } from '@/shared/ui'
import {
  type ReportSummary,
  type ReportTopic,
  type ResultReportData,
  statusOrder,
} from '../model/report'
import { ResultGraph } from './result-graph'
import styles from './result-report.module.scss'

// The one result screen of the learning diagnostic and the exam: the overall
// grade, the conclusion, what went well and what to work on, and the graph of
// the topics in points.
export function ResultReport({
  report,
  summary,
  highlight,
  actions,
  topicExtra,
  children,
}: {
  report: ResultReportData
  summary: ReportSummary
  // Right under the grade: the next step this mode offers.
  highlight?: ReactNode
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
      {highlight}
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
      {checked.length === 0 && (
        <Text as="p" className={styles.lead}>
          {t('resultReport.topicsEmpty')}
        </Text>
      )}
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
