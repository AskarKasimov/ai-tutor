import type { CoursePreparation } from './preparation'

export type Subject = {
  id: string
  name: string
  ready: boolean
  learning_ready: boolean
  exam_ready: boolean
  preparation?: CoursePreparation
}
