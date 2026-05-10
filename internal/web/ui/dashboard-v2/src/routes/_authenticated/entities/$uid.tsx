import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { EntityDetailPane, type EntityTabId } from '@/features/entity/detail-pane'
import type { EntityKind } from '@/lib/queries/entity'

export const Route = createFileRoute('/_authenticated/entities/$uid')({
  validateSearch: (s: Record<string, unknown>) => ({
    kind: (s.kind as EntityKind) ?? 'product',
    tab: (s.tab as EntityTabId) ?? 'main',
  }),
  component: EntityDetail,
})

function EntityDetail() {
  const { uid } = Route.useParams()
  const { kind, tab: initialTab } = Route.useSearch()
  const [tab, setTab] = useState<EntityTabId>(initialTab)

  return (
    <>
      <Header />
      <Main fixed fluid>
        <div className='bg-card h-[calc(100vh-7rem)] overflow-hidden rounded-lg border'>
          <EntityDetailPane
            kind={kind}
            entityId={uid}
            tab={tab}
            onTabChange={setTab}
          />
        </div>
      </Main>
    </>
  )
}
