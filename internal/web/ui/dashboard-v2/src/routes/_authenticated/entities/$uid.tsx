import { createFileRoute } from '@tanstack/react-router'
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
              <EntityListPane kind={kind} selectedId={uid} onSelect={() => {}} />
            </Panel>
            <PanelResizeHandle className='group bg-border hover:bg-primary/40 data-[resize-handle-state=drag]:bg-primary relative w-1 cursor-col-resize transition-colors'>
              <div className='absolute top-1/2 left-1/2 flex -translate-x-1/2 -translate-y-1/2 flex-col gap-0.5 opacity-0 transition-opacity group-hover:opacity-100'>
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
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
