import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { EntityListPane } from '@/features/entity/list-pane'
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
  const navigate = useNavigate()

  return (
    <>
      <Header />
      <Main fixed fluid>
        <div className='bg-card h-[calc(100vh-7rem)] overflow-hidden rounded-lg border'>
          <PanelGroup
            direction='horizontal'
            autoSaveId={`portal-v2.${kind}.panels`}
            className='h-full'
          >
            <Panel defaultSize={22} minSize={15} maxSize={50}>
              <EntityListPane kind={kind} selectedId={uid} onSelect={(id) =>
                navigate({ to: '/entities/$uid', params: { uid: id }, search: { kind, tab: 'main' } })
              } />
            </Panel>
            <PanelResizeHandle className='group relative flex w-2 cursor-col-resize items-center justify-center bg-border transition-colors hover:bg-primary/50 data-[resize-handle-state=drag]:bg-primary'>
              <div className='flex flex-col gap-0.5'>
                {Array.from({ length: 6 }).map((_, i) => (
                  <span key={i} className='block h-0.5 w-3 rounded-full bg-muted-foreground/30 group-hover:bg-foreground/50' />
                ))}
              </div>
            </PanelResizeHandle>
            <Panel defaultSize={78} minSize={40}>
              <EntityDetailPane
                kind={kind}
                entityId={uid}
                tab={tab}
                onTabChange={setTab}
              />
            </Panel>
          </PanelGroup>
        </div>
      </Main>
    </>
  )
}
