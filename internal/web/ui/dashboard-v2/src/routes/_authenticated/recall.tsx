import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/recall')({
  component: () => (
    <TabPlaceholder
      name='Recall'
      description='Search memories + saved conversations + manifest descriptions in one box. Semantic + keyword.'
    />
  ),
})
