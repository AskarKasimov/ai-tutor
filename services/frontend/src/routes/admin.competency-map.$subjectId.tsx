import { createFileRoute } from '@tanstack/react-router'
import { CompetencyMapAdminScreen } from '@/pages/competency-map'

export const Route = createFileRoute('/admin/competency-map/$subjectId')({
  component: CompetencyMapRoute,
})

function CompetencyMapRoute() {
  const { subjectId } = Route.useParams()
  return <CompetencyMapAdminScreen subjectId={subjectId} />
}
