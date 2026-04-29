import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { EntityPage } from '@/features/entity'

// Master-detail Tasks page. Mirrors the products + manifests routes —
// both selection (`id`) and active tab (`tab`) live as search params so
// the URL is shareable / reload-safe and the browser-back stack works.
//
// Renders the generic <EntityPage kind='task'/> — same component the
// /products and /manifests routes use — so the tab strip + master-detail
// layout + keyboard shortcuts stay byte-identical across all three
// surfaces. Schedule + Stats tabs are deferred to follow-up PRs (see
// portal-v2-tabs-restructure scope notes).
const tasksSearch = z.object({
  id: z.string().optional(),
  tab: z
    .enum([
      'main',
      'execution',
      'comments',
      'dependencies',
      'dag',
      'schedule',
      'live_output',
      'stats',
    ])
    .optional()
    .default('main'),
})

export const Route = createFileRoute('/_authenticated/tasks')({
  validateSearch: tasksSearch,
  component: () => <EntityPage kind='task' />,
})
