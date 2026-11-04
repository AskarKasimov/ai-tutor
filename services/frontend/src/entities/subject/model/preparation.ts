import type { Subject } from './subject'
export { isCoursePreparation, isPreparationConsistent } from '@/shared/api'
export type {
  CoursePreparation,
  ExamPreparationState,
  LearningPreparationState,
} from '@/shared/api'

export function preparationBadge(
  subject: Subject,
  direction: 'exam' | 'learn',
):
  | {
      key: string
      color: 'gray' | 'blue' | 'amber' | 'red' | 'green'
      spinning: boolean
    }
  | undefined {
  const preparation = subject.preparation
  if (!preparation) return undefined
  if (direction === 'learn') {
    const state = preparation.learn.state
    return {
      key: `sharedCourse.learn.${state}`,
      color:
        state === 'ready'
          ? 'green'
          : state === 'needs_tasks'
            ? 'amber'
            : 'gray',
      spinning: false,
    }
  }
  const state = preparation.exam.state
  const suffix = subject.exam_ready ? 'available' : 'unavailable'
  return {
    key: `sharedCourse.exam.${state}.${suffix}`,
    color:
      state === 'ready'
        ? 'green'
        : state === 'queued' || state === 'processing'
          ? 'blue'
          : state === 'failed'
            ? 'red'
            : state === 'empty'
              ? 'gray'
              : 'amber',
    spinning: state === 'processing',
  }
}
