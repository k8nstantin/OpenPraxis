import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Search } from 'lucide-react'
import {
  entityKeys,
  useEntityList,
  type EntityKind,
} from '@/lib/queries/entity'
import type { HierarchyNode, Product, Manifest } from '@/lib/types'
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

interface ListRow {
  id: string
  marker: string
  title: string
  status: string
  tags?: string[]
}

// Generic master list pane.
//
// Products mode: tree-style navigator. Each row is expandable via the
// chevron (lazy-loads /api/products/{id}/hierarchy) to drill into
// sub-products N levels deep. Click row → SELECT.
//
// Manifests mode: flat list. Manifests don't have sub-manifests;
// hierarchy is product → manifest → task. The chevron is omitted.
export function EntityListPane({
  kind,
  selectedId,
  onSelect,
}: {
  kind: EntityKind
  selectedId?: string
  onSelect: (id: string) => void
}) {
  const list = useEntityList(kind)
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const rows: ListRow[] = useMemo(() => {
    const data = (list.data ?? []) as (Product | Manifest)[]
    return data.map((d) => ({
      id: d.id,
      marker: d.marker,
      title: d.title,
      status: d.status,
      tags: d.tags,
    }))
  }, [list.data])

  const filtered = useMemo<ListRow[]>(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.marker.toLowerCase().includes(q) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [rows, query])

  // Page size + scroll-driven loadmore. See the products list-pane
  // history for why this is scroll-event and not IntersectionObserver.
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

  const heading = kind === 'product' ? 'Products' : 'Manifests'

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='border-b p-3'>
        <div className='text-muted-foreground mb-2 text-xs font-medium uppercase tracking-wider'>
          {heading}
          <span className='ml-1.5 font-normal opacity-60'>
            {list.isLoading ? '…' : filtered.length}
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
        {list.isLoading ? (
          <div className='space-y-1.5 p-2'>
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className='h-10 w-full' />
            ))}
          </div>
        ) : list.isError ? (
          <div className='p-3 text-xs text-rose-400'>
            Failed: {String(list.error)}
          </div>
        ) : filtered.length === 0 ? (
          <div className='text-muted-foreground p-3 text-xs'>
            {query
              ? 'No matches.'
              : `No ${kind === 'product' ? 'products' : 'manifests'} yet.`}
          </div>
        ) : (
          <div className='py-1'>
            {visible.map((p) => (
              <Row
                key={p.id}
                kind={kind}
                id={p.id}
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

// Tree row. For products, lazy-fetches sub-products on first expand.
// For manifests, the chevron is omitted (flat list).
function Row({
  kind,
  id,
  title,
  status,
  depth,
  expanded,
  selectedId,
  onToggle,
  onSelect,
}: {
  kind: EntityKind
  id: string
  title: string
  status: string
  depth: number
  expanded: Set<string>
  selectedId?: string
  onToggle: (id: string) => void
  onSelect: (id: string) => void
}) {
  const isProduct = kind === 'product'
  const isExpanded = expanded.has(id)
  const hierarchy = useQuery({
    queryKey: entityKeys.hierarchy('product', id),
    queryFn: async () => {
      const res = await fetch(`/api/products/${id}/hierarchy`)
      if (!res.ok) throw new Error(`hierarchy → ${res.status}`)
      return (await res.json()) as HierarchyNode
    },
    enabled: isProduct && isExpanded,
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
        {isProduct ? (
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
        ) : (
          <span className='w-5 shrink-0' />
        )}
        <button
          type='button'
          onClick={() => onSelect(id)}
          className='flex min-w-0 flex-1 items-center justify-between gap-2 py-1.5 text-left'
        >
          <div className='min-w-0'>
            <div className='truncate font-medium'>{title}</div>
            <code className='text-muted-foreground font-mono text-[11px] block truncate'>
              {id}
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
      {isProduct && isExpanded ? (
        hierarchy.isLoading ? (
          <div
            className='text-muted-foreground py-1 text-xs italic'
            style={{ paddingInlineStart: `${(depth + 1) * 14 + 4}px` }}
          >
            loading…
          </div>
        ) : subs.length > 0 ? (
          subs.map((s) => (
            <Row
              key={s.id}
              kind='product'
              id={s.id}
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
