import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { Skeleton } from '@/components/ui/skeleton'
import { UniversalBreadcrumb } from '@/features/entity/breadcrumb'
import { EntityDetailPane, TAB_IDS } from '@/features/entity/detail-pane'
import { useEntityByUid, type EntityKind } from '@/lib/queries/entity'

const entitySearch = z.object({
  tab: z
    .enum(TAB_IDS)
    .optional()
    .default(TAB_IDS[0])
    .catch(TAB_IDS[0]),
})

export const Route = createFileRoute('/_authenticated/entities/$uid')({
  validateSearch: entitySearch,
  component: EntityDetailRoute,
})

function EntityDetailRoute() {
  const { uid } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = useNavigate()
  const { data: entity, isLoading } = useEntityByUid(uid)

  if (isLoading) {
    return (
      <div className='space-y-3 p-6'>
        <Skeleton className='h-8 w-64' />
        <Skeleton className='h-4 w-48' />
        <Skeleton className='h-64 w-full' />
      </div>
    )
  }

  if (!entity) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center'>
        Entity not found
      </div>
    )
  }

  return (
    <EntityDetailPane
      kind={entity.type as EntityKind}
      entityId={entity.entity_uid}
      tab={tab}
      onTabChange={(newTab) =>
        navigate({
          to: '/entities/$uid',
          params: { uid },
          search: { tab: newTab },
        })
      }
      breadcrumb={<UniversalBreadcrumb uid={entity.entity_uid} />}
    />
  )
}
