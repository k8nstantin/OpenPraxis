import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

const manifestsSearch = z.object({
  id: z.string().optional(),
  tab: z.enum(['main', 'execution', 'comments', 'dependencies', 'dag'])
    .optional().default('main').catch('main'),
})

export const Route = createFileRoute('/_authenticated/manifests')({
  validateSearch: manifestsSearch,
  component: () => <EntityPage kind='manifest' />,
})
