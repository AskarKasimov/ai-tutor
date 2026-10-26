import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { CompetencyMapAdminScreen as ExamMaterialsScreen } from '@/pages/competency-map'
import { CompetencyMapAdminScreen as LearningMaterialsScreen } from '@/pages/learning-competency-map'

export const Route = createFileRoute('/admin/courses/$subjectId')({
  validateSearch: z.object({ direction: z.enum(['learn', 'exam']).optional() }),
  component: CourseMaterialsRoute,
})

function CourseMaterialsRoute() {
  const { subjectId } = Route.useParams()
  const { direction } = Route.useSearch()
  return direction === 'exam' ? (
    <ExamMaterialsScreen subjectId={subjectId} />
  ) : (
    <LearningMaterialsScreen subjectId={subjectId} />
  )
}
