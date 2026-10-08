import { createFileRoute } from '@tanstack/react-router'
import { CompetencyMapAdminScreen } from '@/pages/competency-map'

export const Route = createFileRoute('/admin/competency-map')({
  component: CompetencyMapAdminScreen,
})
