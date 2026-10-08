import { createFileRoute } from '@tanstack/react-router'
import { LoginScreen } from '@/pages/login'

export const Route = createFileRoute('/login')({ component: LoginScreen })
