import { useEffect, useMemo, useRef, useState } from 'react'
import { Plus, Search } from 'lucide-react'
import {
  useEntityList,
  useCreateEntity,
  useLiveRuns,
  type EntityKind,
} from '@/lib/queries/entity'
import { useEntityTypes } from '@/lib/queries/entity-types'
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

// Fallback labels for built-in kinds while entity_types loads.
const KIND_LABEL_FALLBACK: Record<string, string> = {
  product: 'Products', manifest: 'Manifests', task: 'Tasks',
  idea: 'Ideas', skill: 'Skills', RAG: 'RAG',
}

interface ListRow { id: string; title: string; status: string; tags?: string[] }

const PAGE_SIZE = 20

export function EntityListPane({
  kind, selectedId, onSelect,
}: {
  kind: EntityKind
  selectedId?: string
  onSelect: (id: string) => void
}) {
  const list = useEntityList(kind)
  const create = useCreateEntity(kind)
  const { data: liveRuns } = useLiveRuns()
  const { data: entityTypes } = useEntityTypes()
  const runningIds = useMemo(
    () => new Set((liveRuns ?? []).map((r) => r.entity_uid)),
    [liveRuns]
  )
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)
  const sentinelRef = useRef<HTMLDivElement>(null)

  // Build label from entity_types display_name when loaded; fall back to
  // static map for built-in kinds, then capitalize the kind name.
  const label = useMemo(() => {
    const et = entityTypes?.find((t) => t.name === kind)
    if (et) return et.display_name + 's'
    const fallback = KIND_LABEL_FALLBACK[kind]
    if (fallback) return fallback
    return kind.charAt(0).toUpperCase() + kind.slice(1) + 's'
  }, [entityTypes, kind])

  // Flat chronological list — newest first (API returns newest first by default)
  const rows: ListRow[] = useMemo(() => {
    const data = (list.data ?? []) as Entity[]
    return data.map((d) => ({
      id: d.entity_uid, title: d.title, status: d.status, tags: d.tags,
    }))
  }, [list.data])

  const filtered = useMemo<ListRow[]>(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter((r) =>
      r.title.toLowerCase().includes(q) ||
      r.id.toLowerCase().includes(q) ||
      (r.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [rows, query])

  useEffect(() => { setVisibleCount(PAGE_SIZE) }, [query])

  const visible = filtered.slice(0, visibleCount)
  const hasMore = visibleCount < filtered.length

  // Infinite scroll sentinel
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
      <div className='space-y-2 border-b p-3'>
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
          <Search className='text-muted-foreground absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2' />
          <Input value={query} onChange={(e) => setQuery(e.target.value)}
            placeholder='Filter…' className='h-8 pl-7 text-sm' />
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
              <button
                key={row.id}
                type='button'
                onClick={() => onSelect(row.id)}
                className={cn(
                  'hover:bg-accent flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm transition-colors',
                  selectedId === row.id && 'bg-accent'
                )}
              >
                {runningIds.has(row.id) ? (
                  <span className='relative mt-0.5 flex h-2 w-2 shrink-0'>
                    <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-violet-400 opacity-75' />
                    <span className='relative inline-flex h-2 w-2 rounded-full bg-violet-500' />
                  </span>
                ) : (
                  <span className={cn(
                    'mt-0.5 h-2 w-2 shrink-0 rounded-full',
                    STATUS_DOT[row.status] ?? 'bg-zinc-400'
                  )} />
                )}
                <span className={cn(
                  'min-w-0 flex-1 truncate',
                  runningIds.has(row.id) && 'text-violet-400 font-medium'
                )}>{row.title}</span>
              </button>
            ))}
            {hasMore && (
              <div ref={sentinelRef}>
                <button
                  type='button'
                  onClick={() => setVisibleCount((n) => Math.min(n + PAGE_SIZE, filtered.length))}
                  className='text-muted-foreground hover:text-foreground hover:bg-accent w-full py-2 text-center text-xs transition-colors'>
                  Load more ({visible.length} of {filtered.length})
                </button>
              </div>
            )}
          </div>
        )}
      </ScrollArea>
    </div>
  )
}
