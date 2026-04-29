import { useEffect } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
// react-resizable-panels v2 — stable, well-documented API. v4 is a
// breaking rewrite with renamed primitives + altered drag semantics.
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import type { EntityKind } from '@/lib/queries/entity'
import { EntityListPane } from './list-pane'
import { EntityDetailPane, type EntityTabId } from './detail-pane'
import { readLastViewedId, writeLastViewedId } from './use-last-viewed'

// Master-detail page for products + manifests. Two panes side-by-side,
// drag-to-resize handle in the middle, autoSaveId persists size in
// localStorage. First visit (no `?id`) → restore last-viewed from
// localStorage so operators come back to where they were.
//
//   ┌──────────────┬─────────────────────────────────────┐
//   │ entity list  │ breadcrumb / title / status         │
//   │ tree/flat    │ ┌─────────────────────────────────┐ │
//   │              │ │ Main · Execution · Comments ·   │ │
//   │              │ │ Dependencies · DAG              │ │
//   │              │ ├─────────────────────────────────┤ │
//   │              │ │  selected tab content           │ │
//   │              │ └─────────────────────────────────┘ │
//   └────────≡─────┴─────────────────────────────────────┘

const DEFAULT_TAB: EntityTabId = 'main'

export function EntityPage({ kind }: { kind: EntityKind }) {
  const route =
    kind === 'product'
      ? '/_authenticated/products'
      : kind === 'task'
        ? '/_authenticated/tasks'
        : '/_authenticated/manifests'
  const targetPath =
    kind === 'product' ? '/products' : kind === 'task' ? '/tasks' : '/manifests'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const search = useSearch({ from: route as any }) as {
    id?: string
    tab?: EntityTabId
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const navigate = useNavigate({ from: route as any })

  const selectedId = search.id
  const tab = (search.tab ?? DEFAULT_TAB) as EntityTabId
  const heading =
    kind === 'product' ? 'Products' : kind === 'task' ? 'Tasks' : 'Manifests'
  const panelGroupId = `portal-v2.${kind}.panels`

  useEffect(() => {
    if (!selectedId) {
      const last = readLastViewedId(kind)
      if (last) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        navigate({
          to: targetPath,
          search: { id: last, tab: DEFAULT_TAB },
        } as any)
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, kind])

  useEffect(() => {
    if (selectedId) {
      writeLastViewedId(kind, selectedId)
    }
  }, [selectedId, kind])

  const setSelected = (id: string) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    navigate({ to: targetPath, search: { id, tab: DEFAULT_TAB } } as any)
  }
  const setTab = (next: EntityTabId) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    navigate({
      to: targetPath,
      search: { id: selectedId, tab: next },
    } as any)
  }

  return (
    <>
      <Header />
      <Main fixed fluid>
        <div className='mb-3 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>{heading}</h1>
        </div>
        <div className='bg-card h-[calc(100vh-10rem)] overflow-hidden rounded-lg border'>
          <PanelGroup
            direction='horizontal'
            autoSaveId={panelGroupId}
            className='h-full'
          >
            <Panel defaultSize={22} minSize={15} maxSize={50}>
              <EntityListPane
                kind={kind}
                selectedId={selectedId}
                onSelect={setSelected}
              />
            </Panel>
            <PanelResizeHandle className='group bg-border hover:bg-primary/40 data-[resize-handle-state=drag]:bg-primary relative w-1 cursor-col-resize transition-colors'>
              {/* Grip indicator — three dots vertically centered. Only
                  visible on hover to keep the resting state clean. */}
              <div className='absolute top-1/2 left-1/2 flex -translate-x-1/2 -translate-y-1/2 flex-col gap-0.5 opacity-0 transition-opacity group-hover:opacity-100'>
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
              </div>
            </PanelResizeHandle>
            <Panel defaultSize={78} minSize={40}>
              <EntityDetailPane
                kind={kind}
                entityId={selectedId}
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
