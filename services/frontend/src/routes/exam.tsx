import { createFileRoute } from '@tanstack/react-router'
import { TrainerScreen } from '@/pages/trainer'
import { z } from 'zod'

export const Route = createFileRoute('/exam')({
  validateSearch: z.object({ subject_id: z.string().optional() }),
  component: () => <TrainerScreen subjectId={Route.useSearch().subject_id} />,
})
