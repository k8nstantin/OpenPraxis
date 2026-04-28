import { useMemo, useState } from 'react'
import { useProductComments } from '@/lib/queries/products'
import type { Comment } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const TYPE_LABEL: Record<string, string> = {
  execution_review: 'Execution Review',
  description_revision: 'Description Revision',
  agent_note: 'Agent Note',
  user_note: 'Note',
  watcher_finding: 'Watcher Finding',
  decision: 'Decision',
  link: 'Link',
}

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}

// Comments tab — full thread with type filter. Newest first. Compose
// surface lands in a follow-up; for now this is read-only.
export function CommentsTab({ productId }: { productId: string }) {
  const comments = useProductComments(productId)
  const [filter, setFilter] = useState<string>('all')

  const types = useMemo(() => {
    if (!comments.data) return [] as string[]
    return Array.from(new Set(comments.data.map((c) => c.type)))
  }, [comments.data])

  const visible = useMemo<Comment[]>(() => {
    if (!comments.data) return []
    const list =
      filter === 'all'
        ? comments.data
        : comments.data.filter((c) => c.type === filter)
    return list.slice().sort((a, b) => {
      const ta = typeof a.created_at === 'number' ? a.created_at : 0
      const tb = typeof b.created_at === 'number' ? b.created_at : 0
      return tb - ta
    })
  }, [comments.data, filter])

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between gap-3'>
        <div className='text-muted-foreground text-sm'>
          {comments.isLoading
            ? 'Loading…'
            : `${visible.length} of ${comments.data?.length ?? 0}`}
        </div>
        <Select value={filter} onValueChange={setFilter}>
          <SelectTrigger className='w-56'>
            <SelectValue placeholder='Filter by type' />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>All types</SelectItem>
            {types.map((t) => (
              <SelectItem key={t} value={t}>
                {TYPE_LABEL[t] ?? t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardContent className='p-0'>
          {comments.isLoading ? (
            <div className='space-y-2 p-4'>
              <Skeleton className='h-16 w-full' />
              <Skeleton className='h-16 w-full' />
            </div>
          ) : comments.isError ? (
            <div className='p-4 text-sm text-rose-400'>
              Failed: {String(comments.error)}
            </div>
          ) : visible.length === 0 ? (
            <div className='text-muted-foreground p-4 text-sm'>
              No comments.
            </div>
          ) : (
            <div className='divide-y'>
              {visible.map((c) => (
                <div key={c.id} className='space-y-1.5 p-4 text-sm'>
                  <div className='flex items-center justify-between gap-2'>
                    <div className='flex items-center gap-2'>
                      <code className='font-mono text-[11px]'>
                        {c.author.slice(0, 16)}
                      </code>
                      <Badge variant='outline' className='text-[10px]'>
                        {TYPE_LABEL[c.type] ?? c.type}
                      </Badge>
                    </div>
                    <span className='text-muted-foreground text-xs'>
                      {fmtTime(c.created_at)}
                    </span>
                  </div>
                  <pre className='font-mono text-xs whitespace-pre-wrap break-words'>
                    {c.body}
                  </pre>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
