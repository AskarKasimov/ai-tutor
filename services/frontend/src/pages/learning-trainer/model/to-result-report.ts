import type { DiagnosticResult } from '@/entities/learning-diagnostic-session'
import {
  type ReportTopic,
  type ResultReportData,
  statusByPoints,
} from '@/widgets/result-report'

// The diagnostic's result as the shared result screen shows it. A main task
// relies on the two basic tasks of its constituent, as in the exam's graph.
export function toResultReport(result: DiagnosticResult): ResultReportData {
  const tasks = [
    ...result.answers.map((a) => a.task),
    ...result.untested_basics,
  ]
  const block = (task: (typeof tasks)[number]) =>
    `${task.competency_id}\u0000${task.constituent_id}`
  const requires = (task: (typeof tasks)[number]) =>
    task.role === 'main'
      ? tasks
          .filter((t) => t.role === 'basic' && block(t) === block(task))
          .map((t) => t.variant_task_id)
      : []
  const asked: ReportTopic[] = result.answers.map((answer) => ({
    id: answer.task.variant_task_id,
    title: answer.task.outcome_name,
    status: answer.skipped
      ? 'gap'
      : statusByPoints(answer.score, answer.grader_max_score),
    score: answer.report_score,
    maxScore: answer.report_max_score,
    assisted: false,
    skipped: answer.skipped ?? false,
    requires: requires(answer.task),
    main: answer.role === 'main',
    // The criteria of a task have no points of their own.
    marks: answer.criterion_results.map((criterion) => ({
      id: criterion.key,
      title: '',
      met: criterion.satisfied,
      reason: criterion.explanation,
      quote: '',
    })),
    // A line sending the student on to the next task has no place in the
    // result; the backend report leaves it out the same way.
    note: answer.feedback
      .filter((line) => !line.toLowerCase().includes('следующ'))
      .join(' '),
  }))
  // Basics are not asked after a full main answer: it credits them.
  const credited: ReportTopic[] = result.untested_basics.map((task) => ({
    id: task.variant_task_id,
    title: task.outcome_name,
    status: 'credited',
    score: null,
    maxScore: null,
    assisted: false,
    skipped: false,
    requires: [],
    main: false,
    marks: [],
    note: '',
  }))
  return {
    grade: result.grade,
    score: result.report_score,
    maxScore: result.report_max_score,
    percent: result.score_percent,
    topics: [...asked, ...credited],
  }
}
