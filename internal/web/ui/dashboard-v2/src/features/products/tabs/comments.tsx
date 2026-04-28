import { useMemo, useState } from 'react'
import {
  useCreateProductComment,
  useProductComments,
} from '@/lib/queries/products'
import type { Comment } from '@/lib/types'
import { Plus } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { MarkdownEditor } from '@/components/markdown-editor'

const TYPE_LABEL: Record<string, string> = {
  execution_review: 'Execution Review',
  description_revision: 'Description Revision',
  agent_note: 'Agent Note',
  user_note: 'Note',
  watcher_finding: 'Watcher Finding',
  decision: 'Decision',
  link: 'Link',
}

// Compose-time type allowlist. The full read filter shows every type
// the API has ever produced (including agent-only ones like
// description_revision, watcher_finding); the compose dropdown is
// narrower so operators only pick types meant for human authoring.
const COMPOSE_TYPES: Array<keyof typeof TYPE_LABEL> = [
  'user_note',
  'decision',
  'agent_note',
]

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}

// Comments tab — full thread + compose. Same MarkdownEditor used
// across description-edit and any future markdown surface (manifests,
// tasks). Cmd-Enter posts; Escape clears.
export function CommentsTab({ productId }: { productId: string }) {
  const comments = useProductComments(productId)
  const create = useCreateProductComment(productId)
  const [filter, setFilter] = useState<string>('all')
  const [composeBody, setComposeBody] = useState('')
  const [composeType, setComposeType] =
    useState<keyof typeof TYPE_LABEL>('user_note')

  // Hide internal change-log records that the Dependencies tab posts
  // (agent_notes wrapped in <dependency_revision>). They live in
  // the Dependencies > Revision history surface — they're operational
  // events, not real comments. Filter at the source so type counts +
  // the filter dropdown don't reflect them either.
  const real = useMemo<Comment[]>(() => {
    if (!comments.data) return []
    return comments.data.filter(
      (c) => !(c.body ?? '').includes('<dependency_revision>')
    )
  }, [comments.data])

  const types = useMemo(() => {
    return Array.from(new Set(real.map((c) => c.type)))
  }, [real])

  const visible = useMemo<Comment[]>(() => {
    const list =
      filter === 'all' ? real : real.filter((c) => c.type === filter)
    return list.slice().sort((a, b) => {
      const ta = typeof a.created_at === 'number' ? a.created_at : 0
      const tb = typeof b.created_at === 'number' ? b.created_at : 0
      return tb - ta
    })
  }, [real, filter])

  const [composing, setComposing] = useState(false)
  const open = () => setComposing(true)
  const close = () => {
    setComposing(false)
    setComposeBody('')
  }
  const post = async () => {
    const body = composeBody.trim()
    if (!body) return
    try {
      await create.mutateAsync({
        author: 'operator',
        type: composeType,
        body,
      })
      close()
    } catch (e) {
      console.error(e)
    }
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-3'>
        <div className='text-muted-foreground text-sm'>
          {comments.isLoading
            ? 'Loading…'
            : `${visible.length} of ${real.length}`}
        </div>
        <div className='flex items-center gap-2'>
          <Select value={filter} onValueChange={setFilter}>
            <SelectTrigger className='h-8 w-48 text-xs'>
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
          {!composing ? (
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={open}
              className='h-8 text-xs'
            >
              <Plus className='mr-1 h-3 w-3' />
              Add comment
            </Button>
          ) : null}
        </div>
      </div>

      {composing ? (
        <Card className='gap-0 py-0'>
          <CardContent className='space-y-2 px-3 py-2'>
            <MarkdownEditor
              value={composeBody}
              onChange={setComposeBody}
              onSave={post}
              onCancel={close}
              compact
              autoFocus
              placeholder='Add a comment in markdown… (Cmd-Enter to post, Esc to cancel)'
            />
            <div className='flex items-center justify-end gap-2'>
              {create.isError ? (
                <span className='mr-auto text-xs text-rose-400'>
                  Post failed: {String(create.error)}
                </span>
              ) : null}
              <Select
                value={composeType}
                onValueChange={(v) =>
                  setComposeType(v as keyof typeof TYPE_LABEL)
                }
              >
                <SelectTrigger className='h-7 w-36 text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {COMPOSE_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>
                      {TYPE_LABEL[t]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={close}
                disabled={create.isPending}
              >
                Cancel
              </Button>
              <Button
                type='button'
                size='sm'
                onClick={post}
                disabled={create.isPending || !composeBody.trim()}
              >
                {create.isPending ? 'Posting…' : 'Post'}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <Card className='gap-0 py-0'>
        <CardContent className='p-0'>
          {comments.isLoading ? (
            <div className='space-y-2 p-3'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
            </div>
          ) : comments.isError ? (
            <div className='p-3 text-sm text-rose-400'>
              Failed: {String(comments.error)}
            </div>
          ) : visible.length === 0 ? (
            <div className='text-muted-foreground p-3 text-sm'>
              No comments.
            </div>
          ) : (
            <div className='divide-y'>
              {visible.map((c) => (
                <div key={c.id} className='space-y-1 p-3 text-sm'>
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
