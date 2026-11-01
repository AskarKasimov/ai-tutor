import type {
  DiagnosticResult,
  TopicResult,
} from '@/entities/diagnostic-session'
import {
  type ReportStatus,
  type ResultReportData,
  statusByPoints,
} from '@/widgets/result-report'

function status(topic: TopicResult): ReportStatus {
  switch (topic.status) {
    case 'gap':
      return 'gap'
    case 'insufficient':
      return 'insufficient'
    case 'credited_by_parent':
      return 'credited'
    case 'unchecked':
      return 'unchecked'
  }
  // Without points only the status is known; an answer that is not a gap is
  // at least partial, whatever its points.
  if (!topic.score || topic.maxScore === null) return 'partial'
  return statusByPoints(topic.score, topic.maxScore)
}

// The exam's result as the shared result screen shows it.
export function toResultReport(result: DiagnosticResult): ResultReportData {
  return {
    grade: result.grade,
    score: result.score,
    maxScore: result.maxScore,
    percent: result.scorePercent,
    topics: result.topics.map((topic) => ({
      id: topic.nodeId,
      title: topic.title,
      status: status(topic),
      score: topic.score,
      maxScore: topic.maxScore,
      assisted: topic.assisted,
      skipped: false,
      requires: topic.requires,
      main: topic.topLevel,
      marks: topic.marks,
      note: topic.reply,
    })),
  }
}
