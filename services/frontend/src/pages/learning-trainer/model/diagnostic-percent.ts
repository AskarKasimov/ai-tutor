import type { DiagnosticProgress } from '@/entities/learning-diagnostic-session'

// Share of the diagnostic passed, in whole percent. Every competency weighs
// 1/N: a main answered well closes it at once, otherwise the main and each
// clarifying basic add a third of it. The value follows the server position,
// which only moves forward, so it never decreases and grows after every task.
export function diagnosticPercent(progress: DiagnosticProgress): number {
  if (progress.status === 'completed') return 100
  const count = progress.competency_count
  const passed = count
    ? (progress.current_competency ?? 1) - 1 + (progress.current_step ?? 0) / 3
    : progress.completed_tasks + progress.skipped_tasks
  const total = count ?? progress.total_tasks
  if (total <= 0) return 0
  // A running diagnostic never shows 100% before its last answer.
  return Math.min(99, Math.max(0, Math.round((passed / total) * 100)))
}
