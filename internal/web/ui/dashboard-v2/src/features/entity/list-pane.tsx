import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, Plus, Search } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import {
  useEntityList,
  useCreateEntity,
  type EntityKind,
} from '@/lib/queries/entity'
import type { Entity } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const STATUS_DOT: Record<string, string> = {
  draft:    'bg-amber-400',
  active:   'bg-emerald-400',
  closed:   'bg-zinc-400',
  archived: 'bg-zinc-600',
}

const STATUS_BADGE: Record<string, string> = {
  draft:    'bg-amber-500/15 text-amber-500',
  active:   'bg-emerald-500/15 text-emerald-500',
  closed:   'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

const KIND_LABEL: Record<string, string> = {
  product: 'Products', manifest: 'Manifests', task: 'Tasks',
  idea: 'Ideas', skill: 'Skills',
}

interface ListRow { id: string; title: string; status: string; tags?: string[] }

// Fetch direct children of any entity via the relationships graph (depth=1).
// A "sub-product", "sub-skill", "sub-idea" is just an entity that depends_on
// or is owned by this entity — same relationships table for all types.
function useEntityChildren(kind: EntityKind, id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['entity-children', kind, id],
    queryFn: async () => {
      const r = await fetch(
        `/api/relationships/graph?root_id=${id}&root_kind=${kind}&depth=1`
      )
      if (!r.ok) return []
      const d = await r.json() as { nodes: { id: string; title: string; kind: string; status: string }[]; edges: unknown[] }
      // Return direct children — all nodes except the root itself
      return (d.nodes ?? []).filter((n) => n.id !== id)
    },
    enabled,
    staleTime: 30_000,
  })
}

export function EntityListPane({
  kind, selectedId, onSelect,
}: {
  kind: EntityKind
  selectedId?: string
  onSelect: (id: string) => void
}) {
  const list = useEntityList(kind)
  const create = useCreateEntity(kind)
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')

  const label = KIND_LABEL[kind] ?? kind

  const rows: ListRow[] = useMemo(() => {
    const data = (list.data ?? []) as Entity[]
    return data.map((d) => ({
      id: d.entity_uid, title: d.title, status: d.status, tags: d.tags,
    }))
  }, [list.data])

  const filtered = useMemo<ListRow[]>(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter((p) =>
      p.title.toLowerCase().includes(q) ||
      p.id.toLowerCase().includes(q) ||
      (p.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [rows, query])

  const PAGE_SIZE = 20
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)
  useEffect(() => { setVisibleCount(PAGE_SIZE) }, [query])
  const visible = filtered.slice(0, visibleCount)
  const hasMore = visibleCount < filtered.length

  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!hasMore) return
    const el = sentinelRef.current
    if (!el) return
    const viewport = el.closest<HTMLElement>('[data-radix-scroll-area-viewport]')
    if (!viewport) return
    const onScroll = () => {
      if (viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 60)
        setVisibleCount((n) => Math.min(n + PAGE_SIZE, filtered.length))
    }
    viewport.addEventListener('scroll', onScroll, { passive: true })
    return () => viewport.removeEventListener('scroll', onScroll)
  }, [hasMore, filtered.length])

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const handleCreate = async () => {
    const title = newTitle.trim()
    if (!title) return
    try {
      const entity = await create.mutateAsync({ type: kind, title, status: 'draft' })
      setNewTitle('')
      setCreating(false)
      if (entity?.entity_uid) onSelect(entity.entity_uid)
    } catch { /* create.isError surfaces it */ }
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='border-b p-3 space-y-2'>
        <div className='flex items-center justify-between'>
          <span className='text-muted-foreground text-xs font-medium uppercase tracking-wider'>
            {label}
            <span className='ml-1.5 font-normal opacity-60'>
              {list.isLoading ? '…' : rows.length}
            </span>
          </span>
          <Button size='sm' variant='ghost' className='h-6 px-2 text-xs'
            onClick={() => { setCreating((v) => !v); setNewTitle('') }}>
            <Plus className='h-3.5 w-3.5 mr-0.5' /> New
          </Button>
        </div>

        {creating && (
          <div className='flex gap-1'>
            <Input autoFocus value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleCreate()
                if (e.key === 'Escape') { setCreating(false); setNewTitle('') }
              }}
              placeholder={`${label.slice(0, -1)} title…`}
              className='h-7 text-xs' />
            <Button size='sm' className='h-7 px-2 text-xs shrink-0'
              onClick={handleCreate}
              disabled={!newTitle.trim() || create.isPending}>
              {create.isPending ? '…' : 'Add'}
            </Button>
          </div>
        )}

        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2' />
          <Input value={query} onChange={(e) => setQuery(e.target.value)}
            placeholder='Filter…' className='pl-7 h-8 text-sm' />
        </div>
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        {list.isLoading ? (
          <div className='space-y-1.5 p-2'>
            {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className='h-10 w-full' />)}
          </div>
        ) : list.isError ? (
          <div className='p-3 text-xs text-rose-400'>Failed to load {label.toLowerCase()}.</div>
        ) : filtered.length === 0 ? (
          <div className='text-muted-foreground p-3 text-xs'>
            {query ? 'No matches.' : `No ${label.toLowerCase()} yet. Click + New to create one.`}
          </div>
        ) : (
          <div className='py-1'>
            {visible.map((row) => (
              <EntityRow key={row.id} kind={kind} row={row} depth={0}
                expanded={expanded} selectedId={selectedId}
                onToggle={toggle} onSelect={onSelect} />
            ))}
            {hasMore && (
              <button
                ref={sentinelRef as unknown as React.Ref<HTMLButtonElement>}
                type='button'
                onClick={() => setVisibleCount((n) => Math.min(n + PAGE_SIZE, filtered.length))}
                className='text-muted-foreground hover:text-foreground hover:bg-accent w-full py-2 text-center text-xs transition-colors'>
                Load more ({visible.length} of {filtered.length})
              </button>
            )}
          </div>
        )}
      </ScrollArea>
    </div>
  )
}

// Single entity row — identical for all entity types.
// Expand arrow loads direct children via relationships graph (same for all types:
// sub-products, sub-skills, sub-ideas all live in the relationships table).
function EntityRow({
  kind, row, depth, expanded, selectedId, onToggle, onSelect,
}: {
  kind: EntityKind
  row: ListRow
  depth: number
  expanded: Set<string>
  selectedId?: string
  onToggle: (id: string) => void
  onSelect: (id: string) => void
}) {
  const isExpanded = expanded.has(row.id)
  const children = useEntityChildren(kind, row.id, isExpanded)
  const hasChildren = isExpanded
    ? (children.data?.length ?? 0) > 0
    : true // show arrow optimistically; hides after expand if empty

  return (
    <div>
      <div
        className={cn(
          'hover:bg-accent flex items-center gap-0.5 pr-2 transition-colors',
          selectedId === row.id && 'bg-accent'
        )}
        style={{ paddingInlineStart: `${depth * 14 + 4}px` }}
      >
        {/* Expand toggle — same for every entity type */}
        <button
          type='button'
          onClick={() => onToggle(row.id)}
          className='text-muted-foreground hover:text-foreground flex h-7 w-5 shrink-0 items-center justify-center'
          aria-label={isExpanded ? 'Collapse' : 'Expand'}
        >
          {isExpanded
            ? <ChevronDown className='h-3.5 w-3.5' />
            : <ChevronRight className='h-3.5 w-3.5' />}
        </button>

        {/* Row content */}
        <button
          type='button'
          onClick={() => onSelect(row.id)}
          className='flex min-w-0 flex-1 items-center gap-2.5 py-2 text-left text-sm'
        >
          {/* Status dot */}
          <span className={cn(
            'mt-0.5 h-2 w-2 shrink-0 rounded-full',
            STATUS_DOT[row.status] ?? 'bg-zinc-400'
          )} />

          {/* Title + UUID + status label */}
          <div className='min-w-0 flex-1'>
            <div className='truncate font-medium leading-snug'>{row.title}</div>
            <div className='flex items-center gap-1.5 mt-0.5'>
              <code className='text-muted-foreground font-mono text-[10px] truncate'>
                {row.id}
              </code>
              <span className={cn(
                'shrink-0 text-[9px] uppercase font-semibold tracking-wide px-1 rounded',
                STATUS_BADGE[row.status] ?? 'bg-zinc-500/15 text-zinc-400'
              )}>
                {row.status}
              </span>
            </div>
          </div>
        </button>
      </div>

      {/* Children — loaded from relationships graph */}
      {isExpanded && (
        children.isLoading ? (
          <div className='text-muted-foreground py-1 text-xs italic'
            style={{ paddingInlineStart: `${(depth + 1) * 14 + 24}px` }}>
            loading…
          </div>
        ) : (children.data?.length ?? 0) === 0 ? (
          <div className='text-muted-foreground py-1 text-xs italic'
            style={{ paddingInlineStart: `${(depth + 1) * 14 + 24}px` }}>
            no sub-{KIND_LABEL[kind]?.toLowerCase().slice(0, -1) ?? kind}s
          </div>
        ) : (
          children.data!.map((child) => (
            <EntityRow
              key={child.id}
              kind={kind}
              row={{ id: child.id, title: child.title, status: child.status }}
              depth={depth + 1}
              expanded={expanded}
              selectedId={selectedId}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ))
        )
      )}
    </div>
  )
}
