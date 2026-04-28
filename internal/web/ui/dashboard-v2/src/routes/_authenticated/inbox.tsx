import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/inbox')({
  component: () => (
    <TabPlaceholder
      name='Inbox'
      description='Comments, markers, PRs threaded by entity.'
    />
  ),
})
