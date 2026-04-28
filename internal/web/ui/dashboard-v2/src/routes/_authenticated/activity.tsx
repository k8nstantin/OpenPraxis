import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/activity')({
  component: () => (
    <TabPlaceholder
      name='Activity'
      description='Raw event log spine — every tool call, commit, state change in chronological order. Filterable, exportable.'
    />
  ),
})
