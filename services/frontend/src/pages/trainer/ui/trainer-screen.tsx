import { LearningHome } from './learning-home'

export function TrainerScreen({ subjectId }: { subjectId?: string }) {
  return <LearningHome requestedSubjectId={subjectId} />
}
