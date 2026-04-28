import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Search } from 'lucide-react'
import { productKeys, useProducts } from '@/lib/queries/products'
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

// Tree-style products navigator. All products listed at root; each
// expandable via chevron to reveal sub-products N levels deep. Click
// a row → SELECT (right pane updates). Click the chevron → EXPAND /
// COLLAPSE without selecting. Same UX as Finder / VS Code's file tree.
export function ProductsListPane({
  selectedId,
  onSelect,
}: {
  selectedId?: string
  onSelect: (id: string) => void
}) {
  const top = useProducts()
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const filtered = useMemo<Product[]>(() => {
    if (!top.data) return []
    const q = query.trim().toLowerCase()
    if (!q) return top.data
    return top.data.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.marker.toLowerCase().includes(q) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [top.data, query])

  // Pagination — show 10 rows initially; a scroll-event handler on
  // the Radix viewport adds 10 more when the operator scrolls within
  // 60px of the bottom. Scroll-driven (not IntersectionObserver) so
  // it only fires when the operator actually scrolls — IO fires on
  // mount when the sentinel is already in view, which cascaded to
  // load every page in a row and defeated the whole point.
  const PAGE_SIZE = 10
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)
  useEffect(() => {
    setVisibleCount(PAGE_SIZE)
  }, [query])
  const visible = filtered.slice(0, visibleCount)
  const hasMore = visibleCount < filtered.length

  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!hasMore) return
    const el = sentinelRef.current
    if (!el) return
    const viewport = el.closest<HTMLElement>(
      '[data-radix-scroll-area-viewport]'
    )
    if (!viewport) return
    const onScroll = () => {
      const remaining =
        viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight
      if (remaining < 60) {
        setVisibleCount((n) => Math.min(n + PAGE_SIZE, filtered.length))
      }
    }
    viewport.addEventListener('scroll', onScroll, { passive: true })
    return () => viewport.removeEventListener('scroll', onScroll)
  }, [hasMore, filtered.length])

  const toggle = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='border-b p-3'>
        <div className='text-muted-foreground mb-2 text-xs font-medium uppercase tracking-wider'>
          Products
          <span className='ml-1.5 font-normal opacity-60'>
            {top.isLoading ? '…' : filtered.length}
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
      <ScrollArea className='min-h-0 flex-1'>
        {top.isLoading ? (
          <div className='space-y-1.5 p-2'>
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className='h-10 w-full' />
            ))}
          </div>
        ) : top.isError ? (
          <div className='p-3 text-xs text-rose-400'>
            Failed: {String(top.error)}
          </div>
        ) : filtered.length === 0 ? (
          <div className='text-muted-foreground p-3 text-xs'>
            {query ? 'No matches.' : 'No products yet.'}
          </div>
        ) : (
          <div className='py-1'>
            {visible.map((p) => (
              <TreeRow
                key={p.id}
                id={p.id}
                marker={p.marker}
                title={p.title}
                status={p.status}
                depth={0}
                expanded={expanded}
                selectedId={selectedId}
                onToggle={toggle}
                onSelect={onSelect}
              />
            ))}
            {hasMore ? (
              // Sentinel is also a button — when content fits in the
              // viewport (no scroll possible), the scroll-event load
              // path can never fire. Click is the always-available
              // fallback. Auto-loads on scroll near bottom too.
              <button
                ref={sentinelRef as unknown as React.Ref<HTMLButtonElement>}
                type='button'
                onClick={() =>
                  setVisibleCount((n) =>
                    Math.min(n + PAGE_SIZE, filtered.length)
                  )
                }
                className='text-muted-foreground hover:text-foreground hover:bg-accent w-full py-2 text-center text-xs transition-colors'
              >
                Load more ({visible.length} of {filtered.length})
              </button>
            ) : filtered.length > PAGE_SIZE ? (
              <div className='text-muted-foreground py-2 text-center text-xs'>
                {filtered.length} shown
              </div>
            ) : null}
          </div>
        )}
      </ScrollArea>
    </div>
  )
}

// Recursive tree row. Lazy-fetches children when first expanded; stays
// cached after collapse so re-expanding is instant.
function TreeRow({
  id,
  marker,
  title,
  status,
  depth,
  expanded,
  selectedId,
  onToggle,
  onSelect,
}: {
  id: string
  marker: string
  title: string
  status: string
  depth: number
  expanded: Set<string>
  selectedId?: string
  onToggle: (id: string) => void
  onSelect: (id: string) => void
}) {
  const isExpanded = expanded.has(id)
  const hierarchy = useQuery({
    queryKey: productKeys.hierarchy(id),
    queryFn: async () => {
      const res = await fetch(`/api/products/${id}/hierarchy`)
      if (!res.ok) throw new Error(`hierarchy → ${res.status}`)
      return (await res.json()) as HierarchyNode
    },
    enabled: isExpanded,
    staleTime: 60 * 1000,
  })
  const subs: HierarchyNode[] = hierarchy.data?.sub_products ?? []

  return (
    <div>
      <div
        className={cn(
          'hover:bg-accent flex items-center gap-1 px-1 text-sm transition-colors',
          selectedId === id && 'bg-accent'
        )}
        style={{ paddingInlineStart: `${depth * 14 + 4}px` }}
      >
        <button
          type='button'
          onClick={() => onToggle(id)}
          className='text-muted-foreground hover:text-foreground flex h-7 w-5 shrink-0 items-center justify-center'
          aria-label={isExpanded ? 'Collapse' : 'Expand'}
        >
          {isExpanded ? (
            <ChevronDown className='h-3.5 w-3.5' />
          ) : (
            <ChevronRight className='h-3.5 w-3.5' />
          )}
        </button>
        <button
          type='button'
          onClick={() => onSelect(id)}
          className='flex min-w-0 flex-1 items-center justify-between gap-2 py-1.5 text-left'
        >
          <div className='min-w-0'>
            <div className='truncate font-medium'>{title}</div>
            <code className='text-muted-foreground font-mono text-[11px]'>
              {marker}
            </code>
          </div>
          <Badge
            variant='secondary'
            className={cn(
              'shrink-0 text-[10px] uppercase',
              STATUS_COLOR[status] ?? 'bg-zinc-500/15'
            )}
          >
            {status}
          </Badge>
        </button>
      </div>
      {isExpanded ? (
        hierarchy.isLoading ? (
          <div
            className='text-muted-foreground py-1 text-xs italic'
            style={{ paddingInlineStart: `${(depth + 1) * 14 + 4}px` }}
          >
            loading…
          </div>
        ) : subs.length > 0 ? (
          subs.map((s) => (
            <TreeRow
              key={s.id}
              id={s.id}
              marker={s.marker}
              title={s.title}
              status={s.status}
              depth={depth + 1}
              expanded={expanded}
              selectedId={selectedId}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ))
        ) : (
          <div
            className='text-muted-foreground py-1 text-xs italic'
            style={{ paddingInlineStart: `${(depth + 1) * 14 + 4}px` }}
          >
            no sub-products
          </div>
        )
      ) : null}
    </div>
  )
}
