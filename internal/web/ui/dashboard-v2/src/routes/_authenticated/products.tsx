import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

const productsSearch = z.object({
  id: z.string().optional(),
  tab: z.enum(['main', 'execution', 'comments', 'dependencies', 'dag'])
    .optional().default('main').catch('main'),
})

export const Route = createFileRoute('/_authenticated/products')({
  validateSearch: productsSearch,
  component: () => <EntityPage kind='product' />,
})
