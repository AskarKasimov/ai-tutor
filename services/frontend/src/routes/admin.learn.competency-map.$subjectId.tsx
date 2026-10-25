import { createFileRoute } from '@tanstack/react-router'
import { CompetencyMapAdminScreen } from '@/pages/learning-competency-map'
export const Route = createFileRoute('/admin/learn/competency-map/$subjectId')({
  component: () => {
    const { subjectId } = Route.useParams()
    return <CompetencyMapAdminScreen subjectId={subjectId} />
  },
})
