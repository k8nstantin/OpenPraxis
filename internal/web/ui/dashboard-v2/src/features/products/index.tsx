import { useEffect } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProductsListPane } from './list-pane'
import { ProductDetailPane } from './detail-pane'
import {
  readLastViewedProductId,
  writeLastViewedProductId,
} from './use-last-viewed'

// Master-detail Products page. Two panes side-by-side:
//
//   ┌──────────────┬─────────────────────────────────────┐
//   │ list of      │ breadcrumb / title / status         │
//   │ current      │ ┌─────────────────────────────────┐ │
//   │ level's      │ │ Main · Desc · ... · DAG         │ │
//   │ children     │ ├─────────────────────────────────┤ │
//   │              │ │  selected tab content           │ │
//   │              │ └─────────────────────────────────┘ │
//   └──────────────┴─────────────────────────────────────┘
//      list (320px)    detail (flex-1)
//
// Click a row in the list → drill in: URL `?id=<id>` updates →
// breadcrumb extends → list swaps to ITS sub-products → detail
// pane shows ITS 6 tabs.
//
// First visit (no `?id`) → restore last-viewed from localStorage so
// operators come back to where they were. If there's no last-viewed,
// the detail pane shows an empty state and the operator picks from
// the list.
type ProductsTab =
  | 'main'
  | 'description'
  | 'stats'
  | 'comments'
  | 'dependencies'
  | 'dag'

export function ProductsPage() {
  const search = useSearch({ from: '/_authenticated/products' })
  const navigate = useNavigate({ from: '/_authenticated/products' })

  const selectedId = search.id
  const tab = (search.tab ?? 'main') as ProductsTab

  // First-load fallback: restore last-viewed product from localStorage
  // when the URL doesn't already specify an id. Operators don't work
  // on 20 products at a time — they live in one and switch.
  useEffect(() => {
    if (!selectedId) {
      const last = readLastViewedProductId()
      if (last) {
        navigate({ search: { id: last, tab: 'main' } })
      }
    }
    // we only want this on mount + when selectedId changes from undefined
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId])

  // Persist whenever the operator picks a new product so a future
  // reload lands them back here.
  useEffect(() => {
    if (selectedId) {
      writeLastViewedProductId(selectedId)
    }
  }, [selectedId])

  const setSelected = (id: string) => {
    navigate({ search: { id, tab: 'main' } })
  }
  const setTab = (next: ProductsTab) => {
    navigate({ search: { id: selectedId, tab: next } })
  }

  return (
    <>
      <Header />
      <Main fixed>
        <div className='mb-3 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Products</h1>
        </div>
        <div className='bg-card grid h-[calc(100vh-10rem)] grid-cols-1 overflow-hidden rounded-lg border lg:grid-cols-[320px_minmax(0,1fr)]'>
          <div className='h-full min-h-0 overflow-hidden border-b lg:border-r lg:border-b-0'>
            <ProductsListPane
              selectedId={selectedId}
              onSelect={setSelected}
            />
          </div>
          <div className='h-full min-h-0 overflow-hidden'>
            <ProductDetailPane
              productId={selectedId}
              tab={tab}
              onTabChange={setTab}
            />
          </div>
        </div>
      </Main>
    </>
  )
}
