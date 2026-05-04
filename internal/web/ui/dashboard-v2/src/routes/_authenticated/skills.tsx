import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

const skillsSearch = z.object({
  id: z.string().optional(),
  tab: z.enum(['main', 'execution', 'comments', 'dependencies', 'dag'])
    .optional().default('main').catch('main'),
})

export const Route = createFileRoute('/_authenticated/skills')({
  validateSearch: skillsSearch,
  component: () => <EntityPage kind='skill' />,
})
