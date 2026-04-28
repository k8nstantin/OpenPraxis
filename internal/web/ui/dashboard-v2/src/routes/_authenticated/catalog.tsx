import { createFileRoute } from '@tanstack/react-router'
import { TabPlaceholder } from '@/features/_placeholder'

export const Route = createFileRoute('/_authenticated/catalog')({
  component: () => (
    <TabPlaceholder
      name='Catalog'
      description='Products → manifests → tasks DAG. Fire from here.'
    />
  ),
})
