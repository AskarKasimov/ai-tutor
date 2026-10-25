import { createFileRoute, Outlet, useRouterState } from '@tanstack/react-router'
import { CompetencyMapAdminScreen } from '@/pages/learning-competency-map'
export const Route = createFileRoute('/admin/learn/competency-map')({
  component: LearningCompetencyMapLayout,
})

function LearningCompetencyMapLayout() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  return pathname === '/admin/learn/competency-map' ? (
    <CompetencyMapAdminScreen />
  ) : (
    <Outlet />
  )
}
