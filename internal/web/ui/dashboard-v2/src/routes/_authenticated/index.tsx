import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/')({
  component: () => (
    <TabPlaceholder
      name='Overview'
      description='Morning landing — alerts, where I left off, overnight ship feed, budget gauge, message of the day.'
    />
  ),
})
