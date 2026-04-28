import { useMemo, useState } from 'react'
import {
  useCreateProductComment,
  useProductComments,
} from '@/lib/queries/products'
import type { Comment } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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

  const post = async () => {
    const body = composeBody.trim()
    if (!body) return
    try {
      await create.mutateAsync({
        author: 'operator',
        type: composeType,
        body,
      })
      setComposeBody('')
    } catch (e) {
      console.error(e)
    }
  }
  const cancel = () => setComposeBody('')

  return (
    <div className='space-y-3'>
      <Card>
        <CardHeader className='py-3'>
          <CardTitle className='text-sm font-medium'>Add comment</CardTitle>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          <div className='space-y-2'>
            <MarkdownEditor
              value={composeBody}
              onChange={setComposeBody}
              onSave={post}
              onCancel={cancel}
              compact
              placeholder='Add a comment in markdown… (Cmd-Enter to post, Esc to clear)'
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
                onClick={cancel}
                disabled={create.isPending || !composeBody}
              >
                Clear
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
          </div>
        </CardContent>
      </Card>

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
