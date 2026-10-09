import { createFileRoute } from '@tanstack/react-router'
import { RoleSelectionScreen } from '@/pages/role-selection'

export const Route = createFileRoute('/welcome')({
  component: RoleSelectionScreen,
})
