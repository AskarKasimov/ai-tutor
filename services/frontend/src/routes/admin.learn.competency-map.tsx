import { createFileRoute } from '@tanstack/react-router'
import { CompetencyMapAdminScreen } from '@/pages/learning-competency-map'
export const Route = createFileRoute('/admin/learn/competency-map')({
  component: CompetencyMapAdminScreen,
})
