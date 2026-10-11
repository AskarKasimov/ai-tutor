import { useEffect, useState } from 'react'
import { Button, Heading, Text } from '@radix-ui/themes'
import {
  AlertCircle,
  AlertTriangle,
  BookOpen,
  CheckCircle2,
  LoaderCircle,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useOverallFeedbackMutation } from '@/features/voice-answer'
import type { OverallFeedbackData } from '@/entities/assessment'
import { buildOverallFeedbackPayload } from '@/features/trainer-session'
import {
  getExpressSteps,
  normalizeText,
  parseStructuredSummary,
  parseGapItem,
  parsePartialItem,
  extractCleanTopicTitle,
} from '@/entities/assessment'
import type { TrainerSession } from '@/entities/trainer-session'
import styles from '@/pages/trainer/ui/trainer-layout.module.scss'

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
        </div>

        {loadingFeedback ? (
          <div className={styles.feedbackLoading}>
            <LoaderCircle
              size={18}
              className={styles.spinner}
              aria-hidden="true"
            />
            <Text>
              Формируем персонализированный фидбэк и рекомендации...
            </Text>
          </div>
        ) : feedback ? (
          (() => {
            const structured = parseStructuredSummary(feedback.summary || '')
            const generalText =
              structured.generalText || normalizeText(feedback.summary || '')
            const goodText = structured.whatIsGood
            const badText = structured.whatNeedsWork

            const recommendations: string[] = []
            if (feedback.recommendations && feedback.recommendations.length > 0) {
              recommendations.push(
                ...feedback.recommendations.slice(0, 3).map(normalizeText),
              )
            } else {
              const steps = getExpressSteps(
                feedback.gaps,
                feedback.partials,
                feedback.recommendations,
              )
              for (const step of steps.slice(0, 3)) {
                recommendations.push(`${step.title}: ${step.note}`)
              }
            }

            return (
              <div className={styles.feedbackContent}>
                {generalText && (
                  <div className={styles.feedbackSummaryText}>
                    {generalText
                      .split(/\n+/)
                      .filter(Boolean)
                      .map((paragraph, idx) => (
                        <p key={idx} className={styles.feedbackParagraph}>
                          {paragraph.trim()}
                        </p>
                      ))}
                  </div>
                )}

                <div className={styles.summaryCardsGrid}>
                  {/* 1. Блок «Что хорошо» */}
                  <div
                    className={`${styles.summaryCard} ${styles.summaryCardGood}`}
                  >
                    <div className={styles.summaryCardHeader}>
                      <CheckCircle2
                        size={18}
                        className={styles.iconSuccess}
                        aria-hidden="true"
                      />
                      <span className={styles.summaryCardTitleGood}>
                        Что хорошо
                      </span>
                    </div>
                    <div className={styles.summaryCardBody}>
                      {goodText ? (
                        <p className={styles.feedbackParagraph}>{goodText}</p>
                      ) : feedback.strengths.length > 0 ? (
                        <ul className={styles.summaryCardList}>
                          {feedback.strengths.slice(0, 3).map((item, idx) => (
                            <li key={idx}>
                              <strong>{normalizeText(item)}</strong>
                            </li>
                          ))}
                        </ul>
                      ) : (
                        <p className={styles.feedbackParagraph}>
                          Базовые понятия поняты верно, продолжайте практиковаться для закрепления уверенности.
                        </p>
                      )}
                    </div>
                  </div>

                  {/* 2. Блок «Что доработать» */}
                  <div
                    className={`${styles.summaryCard} ${styles.summaryCardWarning}`}
                  >
                    <div className={styles.summaryCardHeader}>
                      <AlertTriangle
                        size={18}
                        className={styles.iconWarning}
                        aria-hidden="true"
                      />
                      <span className={styles.summaryCardTitleWarning}>
                        Что доработать
                      </span>
                    </div>
                    <div className={styles.summaryCardBody}>
                      {badText ? (
                        <p className={styles.feedbackParagraph}>{badText}</p>
                      ) : feedback.gaps.length > 0 ? (
                        <ul className={styles.summaryCardList}>
                          {feedback.gaps.slice(0, 3).map((gap, idx) => {
                            const parsed = parseGapItem(gap)
                            const cleanTopic = extractCleanTopicTitle(parsed.title)
                            return (
                              <li key={idx}>
                                <strong>{cleanTopic}:</strong>{' '}
                                {parsed.explanation || 'требуется разобрать ключевую концепцию и критерии'}
                              </li>
                            )
                          })}
                        </ul>
                      ) : feedback.partials.length > 0 ? (
                        <ul className={styles.summaryCardList}>
                          {feedback.partials.slice(0, 3).map((part, idx) => {
                            const parsed = parsePartialItem(part)
                            const cleanTopic = extractCleanTopicTitle(parsed.title)
                            return (
                              <li key={idx}>
                                <strong>{cleanTopic}:</strong>{' '}
                                {parsed.note || 'требуется закрепить обоснование и практическое применение'}
                              </li>
                            )
                          })}
                        </ul>
                      ) : (
                        <p className={styles.feedbackParagraph}>
                          Критических пробелов нет! Все проверенные темы освоены на высоком уровне.
                        </p>
                      )}
                    </div>
                  </div>
                </div>

                {/* 3. Блок «Рекомендации» (кратко расписанные) */}
                {recommendations.length > 0 && (
                  <div className={styles.recommendationsCard}>
                    <div className={styles.recommendationsHeader}>
                      <BookOpen
                        size={18}
                        className={styles.iconRec}
                        aria-hidden="true"
                      />
                      <span className={styles.recommendationsTitle}>
                        Рекомендации
                      </span>
                    </div>
                    <ul className={styles.recommendationsList}>
                      {recommendations.map((rec, idx) => (
                        <li key={idx}>
                          <span className={styles.recNumber}>{idx + 1}</span>
                          <span>{rec}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )
          })()
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
