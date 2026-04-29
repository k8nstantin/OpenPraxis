import { useState } from 'react'
import { Search } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { PickerRow } from './dep-picker'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

// Two-tab modal used by the DAG designer's add-buttons. "Link existing"
// reuses the same picker the textual Dependencies editor uses; "Create
// new" takes a title and fires the create-and-link mutation.
export function LinkOrCreateModal({
  open,
  onOpenChange,
  title,
  description,
  candidates,
  loading,
  emptyLabel,
  onLinkExisting,
  onCreateNew,
  createLabel = 'Create',
}: {
  open: boolean
  onOpenChange: (next: boolean) => void
  title: string
  description: string
  candidates: PickerRow[]
  loading?: boolean
  emptyLabel?: string
  onLinkExisting: (row: PickerRow) => void | Promise<void>
  onCreateNew: (title: string) => void | Promise<void>
  createLabel?: string
}) {
  const [tab, setTab] = useState<'link' | 'create'>('link')
  const [query, setQuery] = useState('')
  const [newTitle, setNewTitle] = useState('')

  const filtered = candidates.filter((r) => {
    const q = query.trim().toLowerCase()
    if (!q) return true
    return r.title.toLowerCase().includes(q) || r.id.toLowerCase().includes(q)
  })

  const reset = () => {
    setQuery('')
    setNewTitle('')
    setTab('link')
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as 'link' | 'create')}
          className='space-y-3'
        >
          <TabsList className='grid w-full grid-cols-2'>
            <TabsTrigger value='link'>Link existing</TabsTrigger>
            <TabsTrigger value='create'>Create new</TabsTrigger>
          </TabsList>
          <TabsContent value='link' className='space-y-2'>
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
                    {query
                      ? 'No matches.'
                      : (emptyLabel ?? 'Nothing to link.')}
                  </div>
                ) : (
                  filtered.map((r) => (
                    <button
                      key={r.id}
                      type='button'
                      onClick={async () => {
                        await onLinkExisting(r)
                        onOpenChange(false)
                        reset()
                      }}
                      className='hover:bg-accent flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm'
                    >
                      <div className='min-w-0'>
                        <div className='truncate font-medium'>{r.title}</div>
                        <code className='text-muted-foreground block truncate font-mono text-[11px]'>
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
          </TabsContent>
          <TabsContent value='create' className='space-y-3'>
            <div className='space-y-1.5'>
              <label className='text-muted-foreground text-xs uppercase tracking-wider'>
                Title
              </label>
              <Input
                autoFocus
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                placeholder='New entity title…'
              />
              <p className='text-muted-foreground text-xs'>
                The new entity will be created in <em>draft</em> status
                and linked to the current parent. You can change its
                status from its detail page.
              </p>
            </div>
            <DialogFooter>
              <Button
                variant='ghost'
                onClick={() => {
                  onOpenChange(false)
                  reset()
                }}
              >
                Cancel
              </Button>
              <Button
                disabled={!newTitle.trim()}
                onClick={async () => {
                  await onCreateNew(newTitle.trim())
                  onOpenChange(false)
                  reset()
                }}
              >
                {createLabel}
              </Button>
            </DialogFooter>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
