import { useEffect, useState } from 'react'
import { Button, Heading, Text } from '@radix-ui/themes'
import {
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  BookOpen,
  CheckCircle2,
  ListChecks,
  LoaderCircle,
  Sparkles,
  Target,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useOverallFeedbackMutation } from '@/features/voice-answer'
import type { OverallFeedbackData } from '@/entities/assessment'
import { buildOverallFeedbackPayload } from '@/features/trainer-session'
import {
  getExpressSteps,
  normalizeText,
  parseGapItem,
  parsePartialItem,
  extractCleanTopicTitle,
} from '@/entities/assessment'
import type { TrainerSession } from '@/entities/trainer-session'
import styles from '@/pages/trainer/ui/trainer-layout.module.scss'

type FeedbackViewMode = 'express' | 'detailed'

export function SessionSummary({
  session,
  onRestart,
  onReview,
  pending,
  error,
}: {
  session: TrainerSession
  onRestart: () => void
  onReview: (id: string) => void
  pending: boolean
  error: boolean
}) {
  const { t } = useTranslation()
  const overallFeedback = useOverallFeedbackMutation(session.userId)
  const total = session.answers.reduce(
    (sum, answer) => sum + answer.assessment.score,
    0,
  )
  const [feedback, setFeedback] = useState<OverallFeedbackData | null>(null)
  const [loadingFeedback, setLoadingFeedback] = useState(true)
  const [feedbackMode, setFeedbackMode] = useState<FeedbackViewMode>('express')

  useEffect(() => {
    let active = true
    const controller = new AbortController()

    const payload = buildOverallFeedbackPayload(session, t)

    overallFeedback
      .mutateAsync({ answers: payload, signal: controller.signal })
      .then((data) => {
        if (active) {
          setFeedback(data)
          setLoadingFeedback(false)
        }
      })
      .catch(() => {
        if (active) setLoadingFeedback(false)
      })

    return () => {
      active = false
      controller.abort()
    }
  }, [overallFeedback.mutateAsync, session, t])

  return (
    <main className={styles.summary}>
      <Text as="p" className={styles.eyebrow}>
        {t('session.summary')}
      </Text>
      <Heading as="h1">{t('session.completed')}</Heading>
      <Text as="p" className={styles.summaryHelp}>
        {t('session.completedHelp')}
      </Text>
      <div className={styles.summaryScore}>
        <Text>{total} / 14</Text>
        <Text>{t('session.totalScore')}</Text>
      </div>

      <section
        className={styles.overallFeedbackSection}
        aria-label="Итоговый педагогический фидбэк"
      >
        <div className={styles.feedbackHeader}>
          <div className={styles.feedbackHeaderTitle}>
            <Sparkles
              size={20}
              className={styles.sparkleIcon}
              aria-hidden="true"
            />
            <Heading as="h2">Педагогический фидбэк по итогам варианта</Heading>
          </div>

          {feedback && (
            <div
              className={styles.feedbackModeToggle}
              role="tablist"
              aria-label="Формат фидбэка"
            >
              <button
                type="button"
                role="tab"
                aria-selected={feedbackMode === 'express'}
                className={`${styles.modeButton} ${feedbackMode === 'express' ? styles.modeButtonActive : ''}`}
                onClick={() => setFeedbackMode('express')}
              >
                <Zap size={14} aria-hidden="true" />
                <span>Краткий разбор</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={feedbackMode === 'detailed'}
                className={`${styles.modeButton} ${feedbackMode === 'detailed' ? styles.modeButtonActive : ''}`}
                onClick={() => setFeedbackMode('detailed')}
              >
                <ListChecks size={14} aria-hidden="true" />
                <span>Подробный разбор</span>
              </button>
            </div>
          )}
        </div>

        {loadingFeedback ? (
          <div className={styles.feedbackLoading}>
            <LoaderCircle
              size={18}
              className={styles.spinner}
              aria-hidden="true"
            />
            <Text>
              Формируем персонализированный фидбэк и план повторения...
            </Text>
          </div>
        ) : feedback ? (
          feedbackMode === 'express' ? (
            <div className={styles.expressContainer}>
              <div className={styles.statBadges}>
                <div
                  className={`${styles.statBadge} ${styles.statBadgeSuccess}`}
                >
                  <CheckCircle2 size={15} aria-hidden="true" />
                  <span>Освоено: {feedback.strengths.length}</span>
                </div>
                {feedback.partials.length > 0 && (
                  <div
                    className={`${styles.statBadge} ${styles.statBadgePartial}`}
                  >
                    <AlertTriangle size={15} aria-hidden="true" />
                    <span>Частично: {feedback.partials.length}</span>
                  </div>
                )}
                <div className={`${styles.statBadge} ${styles.statBadgeGap}`}>
                  <AlertCircle size={15} aria-hidden="true" />
                  <span>Пробелы: {feedback.gaps.length}</span>
                </div>
              </div>

              {feedback.gaps.length > 0 && (
                <div className={styles.expressSection}>
                  <div className={styles.expressSectionTitle}>
                    <Target
                      size={16}
                      className={styles.iconGap}
                      aria-hidden="true"
                    />
                    <Text weight="bold">
                      Пробелы (0/2) — первоочередной фокус (
                      {feedback.gaps.length})
                    </Text>
                  </div>
                  <div className={styles.expressGapsGrid}>
                    {feedback.gaps.map((gap, idx) => {
                      const parsed = parseGapItem(gap)
                      const topic = extractCleanTopicTitle(parsed.title)
                      return (
                        <div key={idx} className={styles.expressGapCard}>
                          <div className={styles.expressGapCardHeader}>
                            <span className={styles.expressGapBadge}>
                              0 / 2
                            </span>
                            <span className={styles.expressGapTopic}>
                              {topic}
                            </span>
                          </div>
                          {parsed.subtopics.length > 0 ? (
                            <div className={styles.expressSubtopics}>
                              <span className={styles.expressSubtopicsLabel}>
                                Что повторить:
                              </span>
                              <span className={styles.expressSubtopicsInline}>
                                {parsed.subtopics.slice(0, 2).join(' · ')}
                              </span>
                            </div>
                          ) : parsed.explanation ? (
                            <div className={styles.expressGapExplanation}>
                              {parsed.explanation}
                            </div>
                          ) : null}
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {feedback.partials.length > 0 && (
                <div className={styles.expressSection}>
                  <div className={styles.expressSectionTitle}>
                    <AlertTriangle
                      size={16}
                      className={styles.iconPartial}
                      aria-hidden="true"
                    />
                    <Text weight="bold">
                      Частично освоенные темы (1/2) — дотянуть до максимума (
                      {feedback.partials.length})
                    </Text>
                  </div>
                  <div className={styles.expressGapsGrid}>
                    {feedback.partials.map((part, idx) => {
                      const parsed = parsePartialItem(part)
                      const topic = extractCleanTopicTitle(parsed.title)
                      return (
                        <div
                          key={idx}
                          className={`${styles.expressGapCard} ${styles.expressPartialCard}`}
                        >
                          <div className={styles.expressGapCardHeader}>
                            <span className={styles.expressPartialBadge}>
                              1 / 2
                            </span>
                            <span className={styles.expressGapTopic}>
                              {topic}
                            </span>
                          </div>
                          <div className={styles.expressPartialNote}>
                            <span className={styles.expressSubtopicsLabel}>
                              Что доработать:
                            </span>
                            <span>
                              {parsed.note ||
                                'Уточнить аргументацию и обосновать выбор своими словами.'}
                            </span>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {feedback.gaps.length === 0 && feedback.partials.length === 0 && (
                <div className={styles.expressSuccessNotice}>
                  <CheckCircle2
                    size={20}
                    className={styles.iconSuccess}
                    aria-hidden="true"
                  />
                  <div>
                    <div className={styles.expressSuccessTitle}>
                      Идеальный результат (14/14)!
                    </div>
                    <div className={styles.expressSuccessSub}>
                      Все темы освоены на максимум. Пробелов не обнаружено.
                    </div>
                  </div>
                </div>
              )}

              <div className={styles.expressSection}>
                <div className={styles.expressSectionTitle}>
                  <ListChecks
                    size={16}
                    className={styles.iconRec}
                    aria-hidden="true"
                  />
                  <Text weight="bold">План действий</Text>
                </div>
                <ol className={styles.expressStepsList}>
                  {getExpressSteps(
                    feedback.gaps,
                    feedback.partials,
                    feedback.recommendations,
                  ).map((step, idx) => (
                    <li key={idx} className={styles.expressStepItem}>
                      <div className={styles.expressStepNumber}>{idx + 1}</div>
                      <div className={styles.expressStepContent}>
                        <div className={styles.expressStepTitle}>
                          {step.title}
                        </div>
                        <div className={styles.expressStepNote}>
                          {step.note}
                        </div>
                      </div>
                    </li>
                  ))}
                </ol>
              </div>

              <div className={styles.expressFooter}>
                <button
                  type="button"
                  className={styles.switchDetailedButton}
                  onClick={() => setFeedbackMode('detailed')}
                >
                  <span>Перейти к подробному разбору с пояснением ошибок</span>
                  <ArrowRight size={14} aria-hidden="true" />
                </button>
              </div>
            </div>
          ) : (
            <div className={styles.feedbackContent}>
              {feedback.summary && (
                <div className={styles.feedbackSummaryText}>
                  {normalizeText(feedback.summary)
                    .split(/\n+/)
                    .filter(Boolean)
                    .map((paragraph, idx) => (
                      <p key={idx} className={styles.feedbackParagraph}>
                        {paragraph.trim()}
                      </p>
                    ))}
                </div>
              )}

              <div className={styles.feedbackGroup}>
                <div className={styles.feedbackGroupTitle}>
                  <CheckCircle2
                    size={16}
                    className={styles.iconSuccess}
                    aria-hidden="true"
                  />
                  <Text weight="bold">Сильные стороны</Text>
                </div>
                {feedback.strengths.length > 0 ? (
                  <ul className={styles.feedbackList}>
                    {feedback.strengths.map((str, idx) => (
                      <li key={idx} className={styles.feedbackItem}>
                        <Text>{normalizeText(str)}</Text>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <Text className={styles.emptyNote}>
                    В этой попытке пока нет тем с максимальным баллом (2/2).
                    Проработайте темы с пробелами ниже для закрепления знаний.
                  </Text>
                )}
              </div>

              {feedback.partials.length > 0 && (
                <div className={styles.feedbackGroup}>
                  <div className={styles.feedbackGroupTitle}>
                    <AlertTriangle
                      size={16}
                      className={styles.iconPartial}
                      aria-hidden="true"
                    />
                    <Text weight="bold">Частично освоенные темы</Text>
                  </div>
                  <ul className={styles.feedbackList}>
                    {feedback.partials.map((part, idx) => (
                      <li key={idx} className={styles.feedbackItem}>
                        <Text>{normalizeText(part)}</Text>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {feedback.gaps.length > 0 && (
                <div className={styles.feedbackGroup}>
                  <div className={styles.feedbackGroupTitle}>
                    <AlertCircle
                      size={16}
                      className={styles.iconGap}
                      aria-hidden="true"
                    />
                    <Text weight="bold">Темы, требующие повторения</Text>
                  </div>
                  <div className={styles.gapCardsList}>
                    {feedback.gaps.map((gap, idx) => {
                      const parsed = parseGapItem(gap)
                      return (
                        <div key={idx} className={styles.gapCardItem}>
                          <div className={styles.gapTitle}>
                            <Text weight="bold">{parsed.title}</Text>
                          </div>
                          {parsed.explanation && (
                            <div className={styles.gapMistakeRow}>
                              <span className={styles.gapLabel}>
                                В чём ошибка
                              </span>
                              <span className={styles.gapValue}>
                                {parsed.explanation}
                              </span>
                            </div>
                          )}
                          {parsed.subtopics.length > 0 && (
                            <div className={styles.gapRepeatBlock}>
                              <span className={styles.gapLabel}>
                                Что повторить по теме:
                              </span>
                              <ul className={styles.gapSubtopicsList}>
                                {parsed.subtopics.map((sub, sIdx) => (
                                  <li key={sIdx}>{sub}</li>
                                ))}
                              </ul>
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {feedback.recommendations.length > 0 && (
                <div className={styles.feedbackGroup}>
                  <div className={styles.feedbackGroupTitle}>
                    <BookOpen
                      size={16}
                      className={styles.iconRec}
                      aria-hidden="true"
                    />
                    <Text weight="bold">
                      Рекомендации по дальнейшему обучению
                    </Text>
                  </div>
                  <ul className={styles.feedbackList}>
                    {feedback.recommendations.map((rec, idx) => (
                      <li key={idx} className={styles.feedbackItem}>
                        <Text>{normalizeText(rec)}</Text>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              <div className={styles.expressFooter}>
                <button
                  type="button"
                  className={styles.switchDetailedButton}
                  onClick={() => setFeedbackMode('express')}
                >
                  <span>← Вернуться к краткому разбору</span>
                </button>
              </div>
            </div>
          )
        ) : null}
      </section>

      <ol className={styles.summaryList}>
        {session.tasks.map((task, i) => {
          const answer = session.answers.find(
            (item) => item.assignmentId === task.assignmentId,
          )
          return (
            <li key={task.assignmentId}>
              <button
                onClick={() => onReview(task.assignmentId)}
                disabled={pending}
              >
                <span className={styles.summaryNumber}>
                  {String(i + 1).padStart(2, '0')}
                </span>
                <span className={styles.summaryQuestion}>
                  {t(task.questionKey)}
                </span>
                <span className={styles.summaryAnswerScore}>
                  {answer?.assessment.score} / 2
                </span>
              </button>
            </li>
          )
        })}
      </ol>
      {error && (
        <Text as="p" role="alert" className={styles.speechError}>
          {t('session.loadError')}
        </Text>
      )}
      <Button className={styles.primary} disabled={pending} onClick={onRestart}>
        {t(pending ? 'session.loading' : 'session.restart')}
      </Button>
    </main>
  )
}
