import { useMemo, useState } from 'react'
import { ChevronRight, Search } from 'lucide-react'
import {
  useProductHierarchy,
  useProducts,
} from '@/lib/queries/products'
import type { HierarchyNode, Product } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
  cancelled: 'bg-rose-500/15 text-rose-500',
}

// Left-pane navigator. Shows products at the current level only —
// when no product is selected: every product on the node (operators
// pick the umbrella to enter); when a product IS selected: that
// product's `sub_products`. Manifests deliberately do NOT appear
// here — they get their own top-level menu later.
//
// Selection + drill semantics live in the parent ProductsPage; this
// component is purely presentational + raises onSelect when the
// operator clicks a row.
export function ProductsListPane({
  selectedId,
  onSelect,
}: {
  selectedId?: string
  onSelect: (id: string) => void
}) {
  // Show top-level products when nothing is selected; otherwise show
  // the selected product's sub-products. Both endpoints feed the same
  // shape (HierarchyNode | Product) — we normalize to a thin row.
  const top = useProducts()
  const drill = useProductHierarchy(selectedId)
  const [query, setQuery] = useState('')

  const rows: Row[] = useMemo(() => {
    if (selectedId && drill.data) {
      return (drill.data.sub_products ?? []).map(rowFromHierarchy)
    }
    if (top.data) {
      return top.data.map(rowFromProduct)
    }
    return []
  }, [selectedId, drill.data, top.data])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter(
      (r) =>
        r.title.toLowerCase().includes(q) ||
        r.marker.toLowerCase().includes(q)
    )
  }, [rows, query])

  const isLoading = selectedId ? drill.isLoading : top.isLoading
  const isError = selectedId ? drill.isError : top.isError
  const error = selectedId ? drill.error : top.error

  const heading = selectedId ? 'Sub-products' : 'All products'

  return (
    <div className='flex h-full flex-col'>
      <div className='border-b p-3'>
        <div className='text-muted-foreground mb-2 text-xs font-medium uppercase tracking-wider'>
          {heading}
          <span className='ml-1.5 font-normal opacity-60'>
            {isLoading ? '…' : filtered.length}
          </span>
        </div>
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2' />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder='Filter…'
            className='pl-8'
          />
        </div>
      </div>
      <ScrollArea className='flex-1'>
        {isLoading ? (
          <div className='space-y-1.5 p-2'>
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className='h-12 w-full' />
            ))}
          </div>
        ) : isError ? (
          <div className='p-3 text-xs text-rose-400'>
            Failed: {String(error)}
          </div>
        ) : filtered.length === 0 ? (
          <div className='text-muted-foreground p-3 text-xs'>
            {selectedId
              ? 'No sub-products. This product is a leaf.'
              : query
                ? 'No matches.'
                : 'No products yet.'}
          </div>
        ) : (
          <div className='py-1'>
            {filtered.map((r) => (
              <button
                key={r.id}
                type='button'
                onClick={() => onSelect(r.id)}
                className={cn(
                  'group flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
                  'hover:bg-accent',
                  selectedId === r.id && 'bg-accent'
                )}
              >
                <div className='min-w-0 flex-1'>
                  <div className='flex items-center gap-2'>
                    <span className='truncate font-medium'>{r.title}</span>
                  </div>
                  <code className='text-muted-foreground font-mono text-[11px]'>
                    {r.marker}
                  </code>
                </div>
                <Badge
                  variant='secondary'
                  className={cn(
                    'shrink-0 text-[10px] uppercase',
                    STATUS_COLOR[r.status] ?? 'bg-zinc-500/15'
                  )}
                >
                  {r.status}
                </Badge>
                <ChevronRight className='text-muted-foreground h-3.5 w-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-60' />
              </button>
            ))}
          </div>
        )}
      </ScrollArea>
    </div>
  )
}

type Row = {
  id: string
  marker: string
  title: string
  status: string
}

function rowFromProduct(p: Product): Row {
  return {
    id: p.id,
    marker: p.marker,
    title: p.title,
    status: p.status,
  }
}

function rowFromHierarchy(h: HierarchyNode): Row {
  return {
    id: h.id,
    marker: h.marker,
    title: h.title,
    status: h.status,
  }
}
