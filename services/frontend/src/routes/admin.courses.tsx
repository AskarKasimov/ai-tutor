import { createFileRoute, Outlet, useRouterState } from '@tanstack/react-router'
import { AdminCoursesScreen } from '@/pages/admin-courses'

export const Route = createFileRoute('/admin/courses')({
  component: AdminCoursesLayout,
})

function AdminCoursesLayout() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  return pathname === '/admin/courses' ? <AdminCoursesScreen /> : <Outlet />
}
