import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

const skillsSearch = z.object({
  id: z.string().optional(),
  tab: z.enum(['main', 'execution', 'runs', 'comments', 'dependencies', 'dag'])
    .optional().default('main').catch('main'),
})

export const Route = createFileRoute('/_authenticated/skills')({
  validateSearch: skillsSearch,
  beforeLoad: ({ search }) => {
    if (search.id) {
      throw redirect({ to: '/entities/$uid', params: { uid: search.id } })
    }
  },
  component: () => <EntityPage kind='skill' />,
})
