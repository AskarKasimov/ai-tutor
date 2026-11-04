export type ExamPreparationState =
  | 'empty'
  | 'queued'
  | 'processing'
  | 'failed'
  | 'needs_metadata'
  | 'needs_build'
  | 'ready'
export type LearningPreparationState = 'empty' | 'needs_tasks' | 'ready'
export type CoursePreparation = {
  exam: {
    state: ExamPreparationState
    source: 'materials' | 'competency_map' | null
    stale: boolean
  }
  learn: { state: LearningPreparationState }
}

export function isCoursePreparation(
  value: unknown,
): value is CoursePreparation {
  if (!value || typeof value !== 'object') return false
  const preparation = value as Partial<CoursePreparation>
  return (
    !!preparation.exam &&
    [
      'empty',
      'queued',
      'processing',
      'failed',
      'needs_metadata',
      'needs_build',
      'ready',
    ].includes(preparation.exam.state) &&
    (preparation.exam.source === null ||
      preparation.exam.source === 'materials' ||
      preparation.exam.source === 'competency_map') &&
    typeof preparation.exam.stale === 'boolean' &&
    !!preparation.learn &&
    ['empty', 'needs_tasks', 'ready'].includes(preparation.learn.state)
  )
}

export function isPreparationConsistent(subject: {
  exam_ready: boolean
  learning_ready: boolean
  preparation?: CoursePreparation
}): boolean {
  const preparation = subject.preparation
  if (!preparation) return false
  if (preparation.exam.state === 'ready' && !subject.exam_ready) return false
  if ((preparation.learn.state === 'ready') !== subject.learning_ready)
    return false
  if (preparation.exam.state === 'empty')
    return preparation.exam.source === null
  return preparation.exam.source !== null
}
