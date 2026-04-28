import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/productivity')({
  component: () => (
    <TabPlaceholder
      name='Productivity'
      description='KPIs + comparison views: per-developer, per-agent, per-product. Cycles-to-acceptance, $/PR, output/cost ratio.'
    />
  ),
})
