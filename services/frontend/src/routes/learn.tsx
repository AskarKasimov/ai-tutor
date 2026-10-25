import { createFileRoute } from '@tanstack/react-router'
import { TrainerScreen } from '@/pages/learning-trainer'
export const Route = createFileRoute('/learn')({ component: TrainerScreen })
