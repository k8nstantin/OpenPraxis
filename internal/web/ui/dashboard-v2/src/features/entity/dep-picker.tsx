import { useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// Reusable search-picker over a list of {id, title, status}
// rows. Caller supplies the data + onPick handler. Filters by title,
// id, or any other text in the searchable string.
//
// Used by the Dependencies editor's Add buttons (upstream / downstream
// / manifest pickers).
export interface PickerRow {
  id: string
  title: string
  status: string
}

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

export function DepPicker({
  open,
  onOpenChange,
  title,
  description,
  rows,
  loading,
  emptyLabel,
  onPick,
}: {
  open: boolean
  onOpenChange: (next: boolean) => void
  title: string
  description: string
  rows: PickerRow[]
  loading?: boolean
  emptyLabel?: string
  onPick: (row: PickerRow) => void | Promise<void>
}) {
  const [query, setQuery] = useState('')
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter(
      (r) =>
        r.title.toLowerCase().includes(q) ||
        r.id.toLowerCase().includes(q)
    )
  }, [rows, query])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2' />
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder='Filter…'
            className='pl-8'
          />
        </div>
        <ScrollArea className='max-h-72'>
          <div className='py-1'>
            {loading ? (
              <div className='space-y-1.5 p-1'>
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className='h-10 w-full' />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <div className='text-muted-foreground p-3 text-xs'>
                {query ? 'No matches.' : (emptyLabel ?? 'Nothing to pick.')}
              </div>
            ) : (
              filtered.map((r) => (
                <button
                  key={r.id}
                  type='button'
                  onClick={() => {
                    void onPick(r)
                    onOpenChange(false)
                  }}
                  className='hover:bg-accent flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm'
                >
                  <div className='min-w-0'>
                    <div className='truncate font-medium'>{r.title}</div>
                    <code className='text-muted-foreground font-mono text-[11px] block truncate'>
                      {r.id}
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
                </button>
              ))
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
