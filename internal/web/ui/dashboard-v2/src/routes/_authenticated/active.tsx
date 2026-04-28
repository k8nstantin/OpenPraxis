import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/active')({
  component: () => (
    <TabPlaceholder
      name='Active'
      description='Live tail of running agents + the operator action queue.'
    />
  ),
})
