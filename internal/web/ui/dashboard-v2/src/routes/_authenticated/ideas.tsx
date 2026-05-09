import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

const ideasSearch = z.object({
  id: z.string().optional(),
  tab: z.enum(['main', 'execution', 'runs', 'comments', 'dependencies', 'dag'])
    .optional().default('main').catch('main'),
})

export const Route = createFileRoute('/_authenticated/ideas')({
  validateSearch: ideasSearch,
  component: () => <EntityPage kind='idea' />,
})
