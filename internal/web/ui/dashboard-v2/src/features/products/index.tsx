import { useEffect } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProductsListPane } from './list-pane'
import { ProductDetailPane, type ProductsTabId } from './detail-pane'
import {
  readLastViewedProductId,
  writeLastViewedProductId,
} from './use-last-viewed'

// Master-detail Products page. Two panes side-by-side, separated by a
// drag-to-resize handle. Operator drags the divider left/right to
// reshape; size persists in localStorage so the layout sticks.
//
//   ┌──────────────┬─────────────────────────────────────┐
//   │ products     │ breadcrumb / title / status         │
//   │ tree         │ ┌─────────────────────────────────┐ │
//   │ (drill-in)   │ │ Description · Comments ·        │ │
//   │              │ │ Dependencies · DAG · Stats      │ │
//   │              │ ├─────────────────────────────────┤ │
//   │              │ │  selected tab content           │ │
//   │              │ └─────────────────────────────────┘ │
//   └────────≡─────┴─────────────────────────────────────┘
//        list (resizable)   detail (resizable)
//
// First visit (no `?id`) → restore last-viewed from localStorage so
// operators come back to where they were.

const DEFAULT_TAB: ProductsTabId = 'description'
const PANEL_GROUP_ID = 'portal-v2.products.panels'

export function ProductsPage() {
  const search = useSearch({ from: '/_authenticated/products' })
  const navigate = useNavigate({ from: '/_authenticated/products' })

  const selectedId = search.id
  const tab = (search.tab ?? DEFAULT_TAB) as ProductsTabId

  useEffect(() => {
    if (!selectedId) {
      const last = readLastViewedProductId()
      if (last) {
        navigate({
          to: '/products',
          search: { id: last, tab: DEFAULT_TAB },
        })
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId])

  useEffect(() => {
    if (selectedId) {
      writeLastViewedProductId(selectedId)
    }
  }, [selectedId])

  const setSelected = (id: string) => {
    navigate({ to: '/products', search: { id, tab: DEFAULT_TAB } })
  }
  const setTab = (next: ProductsTabId) => {
    navigate({ to: '/products', search: { id: selectedId, tab: next } })
  }

  return (
    <>
      <Header />
      <Main fixed fluid>
        <div className='mb-3 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Products</h1>
        </div>
        <div className='bg-card h-[calc(100vh-10rem)] overflow-hidden rounded-lg border'>
          <PanelGroup
            direction='horizontal'
            autoSaveId={PANEL_GROUP_ID}
            className='h-full'
          >
            <Panel defaultSize={22} minSize={15} maxSize={50}>
              <ProductsListPane
                selectedId={selectedId}
                onSelect={setSelected}
              />
            </Panel>
            <PanelResizeHandle className='bg-border hover:bg-primary/50 data-[resize-handle-active=true]:bg-primary w-px transition-colors' />
            <Panel defaultSize={78} minSize={40}>
              <ProductDetailPane
                productId={selectedId}
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
