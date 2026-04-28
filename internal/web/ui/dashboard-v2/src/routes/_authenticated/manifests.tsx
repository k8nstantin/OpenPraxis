import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

// Master-detail Manifests page. Mirrors the products route shape — both
// selection (`id`) and active tab (`tab`) live as search params so the
// URL is shareable / reload-safe and the browser-back stack works.
//
// Renders the generic <EntityPage kind='manifest'/> — same component
// the /products route uses — so the tab strip + master-detail layout
// + keyboard shortcuts stay byte-identical between the two surfaces.
const manifestsSearch = z.object({
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

export const Route = createFileRoute('/_authenticated/manifests')({
  validateSearch: manifestsSearch,
  component: () => <EntityPage kind='manifest' />,
})
