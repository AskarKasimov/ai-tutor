import { createFileRoute } from '@tanstack/react-router'
import { AdminCoursesScreen } from '@/pages/admin-courses'

export const Route = createFileRoute('/admin/courses')({
  component: AdminCoursesScreen,
})
