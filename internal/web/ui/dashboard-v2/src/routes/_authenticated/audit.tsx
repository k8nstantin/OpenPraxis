import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/audit')({
  component: () => (
    <TabPlaceholder
      name='Audit'
      description='Compliance + violations stream — visceral / amnesia / delusion / force-push / no-verify / budget breach. Persistent banners until acked.'
    />
  ),
})
