import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

// Master-detail Products page. Both selection (`id`) and active tab
// (`tab`) live as search params so the URL is shareable / reload-safe
// and the browser-back stack actually works.
//
// Renders the generic <EntityPage kind='product'/> — same component
// the /manifests route uses — so the tab strip + master-detail layout
// stay byte-identical between the two surfaces.
const productsSearch = z.object({
  id: z.string().optional(),
  tab: z
    .enum([
      'main',
      'execution',
      'comments',
      'dependencies',
      'dag',
      'schedule',
      'stats',
    ])
    .optional()
    .default('main'),
})

export const Route = createFileRoute('/_authenticated/products')({
  validateSearch: productsSearch,
  component: () => <EntityPage kind='product' />,
})
