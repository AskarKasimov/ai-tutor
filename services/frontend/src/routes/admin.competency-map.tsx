import { createFileRoute } from '@tanstack/react-router'
import { Navigate, Outlet, useRouterState } from '@tanstack/react-router'

export const Route = createFileRoute('/admin/competency-map')({
  component: CompetencyMapLayout,
})

function CompetencyMapLayout() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  if (pathname === '/admin/competency-map')
    return <Navigate to="/admin/courses" replace />
  return <Outlet />
}
