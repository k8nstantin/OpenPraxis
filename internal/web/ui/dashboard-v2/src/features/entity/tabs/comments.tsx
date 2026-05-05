import { useMemo, useState } from 'react'
import {
  useCreateEntityComment,
  useEntityComments,
  useUpdateEntity,
  type EntityKind,
} from '@/lib/queries/entity'
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
import { cn } from '@/lib/utils'
import { useQueryClient } from '@tanstack/react-query'
import { CommentAttachments } from '@/components/comment-attachments'
import { AttachmentComposer, usePendingFiles } from '@/components/content-block'
import { uploadAttachment, attachmentKeys } from '@/lib/queries/attachments'

const TYPE_LABEL: Record<string, string> = {
  execution_review:    'Execution Review',
  description_revision:'Description',
  agent_note:          'Agent Note',
  user_note:           'Note',
  watcher_finding:     'Watcher Finding',
  decision:            'Decision',
  link:                'Link',
}

const COMPOSE_TYPES = [
  'user_note',
  'execution_review',
  'agent_note',
  'decision',
  'description_revision',
] as const

function fmtTime(v: string | number | undefined): string {
  if (!v) return ''
  try {
    const d = typeof v === 'number' ? new Date(v) : new Date(v)
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  } catch { return String(v) }
}

export function CommentsTab({ kind, entityId }: { kind: EntityKind; entityId: string }) {
  const qc = useQueryClient()
  const comments = useEntityComments(kind, entityId)
  const create = useCreateEntityComment(kind, entityId)
  const update = useUpdateEntity(kind, entityId)

  const [filter, setFilter] = useState('all')
  const [composeType, setComposeType] = useState<keyof typeof TYPE_LABEL>('user_note')
  const [composing, setComposing] = useState(false)
  const [body, setBody] = useState('')
  const [posting, setPosting] = useState(false)
  const files = usePendingFiles()

  const real = useMemo<Comment[]>(() => {
    if (!comments.data) return []
    return comments.data.filter(c => !(c.body ?? '').includes('<dependency_revision>'))
  }, [comments.data])

  const types = useMemo(() => Array.from(new Set(real.map(c => c.type))), [real])

  const visible = useMemo<Comment[]>(() => {
    const list = filter === 'all' ? real : real.filter(c => c.type === filter)
    return list.slice().sort((a, b) => {
      const ta = typeof a.created_at === 'number' ? a.created_at : 0
      const tb = typeof b.created_at === 'number' ? b.created_at : 0
      return tb - ta
    })
  }, [real, filter])

  const close = () => { setComposing(false); setBody(''); files.clear() }

  const post = async () => {
    const text = body.trim()
    if (!text && files.pending.length === 0) return
    setPosting(true)
    try {
      const created = await create.mutateAsync({
        author: 'operator',
        type: composeType,
        body: text || '(attachment only)',
      }) as { id?: string } | undefined

      const newId = created?.id
      if (newId) {
        for (const p of files.pending) {
          try {
            const aid = await uploadAttachment(p.file)
            await fetch(`/api/attachments/${aid}/claim`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ comment_id: newId }),
            })
          } catch { /* non-fatal */ }
        }
        qc.invalidateQueries({ queryKey: attachmentKeys.byComment(newId) })
      }

      // Description-as-comment: also PATCH the entity description field
      if (composeType === 'description_revision' && text) {
        update.mutate({ description: text } as Parameters<typeof update.mutate>[0])
      }

      close()
    } catch (e) {
      console.error(e)
    } finally {
      setPosting(false)
    }
  }

  return (
    <div className='space-y-3'>

      {/* Toolbar */}
      <div className='flex items-center justify-between gap-3'>
        <span className='text-muted-foreground text-sm'>
          {comments.isLoading ? 'Loading…' : `${visible.length} of ${real.length}`}
        </span>
        <div className='flex items-center gap-2'>
          <Select value={filter} onValueChange={setFilter}>
            <SelectTrigger className='h-8 w-44 text-xs'>
              <SelectValue placeholder='Filter by type' />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>All types</SelectItem>
              {types.map(t => (
                <SelectItem key={t} value={t}>{TYPE_LABEL[t] ?? t}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!composing && (
            <Button type='button' size='sm' variant='outline' onClick={() => setComposing(true)} className='h-8 text-xs'>
              <Plus className='mr-1 h-3 w-3' />Add comment
            </Button>
          )}
        </div>
      </div>

      {/* Composer */}
      {composing && (
        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <Select value={composeType} onValueChange={v => setComposeType(v as keyof typeof TYPE_LABEL)}>
              <SelectTrigger className='h-7 w-44 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {COMPOSE_TYPES.map(t => (
                  <SelectItem key={t} value={t}>{TYPE_LABEL[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button type='button' variant='ghost' size='sm' className='h-7 px-2 text-xs ml-auto' onClick={close}>Cancel</Button>
          </div>
          <AttachmentComposer
            body={body}
            onBodyChange={setBody}
            pending={files.pending}
            onAdd={files.add}
            onRemove={files.remove}
            onSubmit={post}
            placeholder={
              composeType === 'description_revision'
                ? 'Write description… Markdown supported, drag/paste files (Cmd-Enter to post)'
                : 'Add a comment… drag/paste files to attach (Cmd-Enter to post)'
            }
            submitLabel={
              composeType === 'description_revision' ? 'Post as Description' : 'Post'
            }
            submitting={posting}
          />
          {create.isError && (
            <p className='text-xs text-rose-400'>Post failed: {String(create.error)}</p>
          )}
        </div>
      )}

      {/* Comment list */}
      <Card className='gap-0 py-0'>
        <CardContent className='p-0'>
          {comments.isLoading ? (
            <div className='space-y-2 p-3'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
            </div>
          ) : comments.isError ? (
            <div className='p-3 text-sm text-rose-400'>Failed: {String(comments.error)}</div>
          ) : visible.length === 0 ? (
            <div className='text-muted-foreground p-3 text-sm'>No comments yet.</div>
          ) : (
            <div className='divide-y'>
              {visible.map(c => (
                <div
                  key={c.id}
                  className={cn(
                    'space-y-2 p-3 text-sm',
                    c.type === 'description_revision' && 'border-l-2 border-emerald-400/60 bg-emerald-400/5',
                  )}
                >
                  <div className='flex items-center justify-between gap-2'>
                    <div className='flex items-center gap-2'>
                      <code className='font-mono text-[11px]'>{c.author.slice(0, 16)}</code>
                      <Badge
                        variant={c.type === 'description_revision' ? 'secondary' : 'outline'}
                        className='text-[10px]'
                      >
                        {TYPE_LABEL[c.type] ?? c.type}
                      </Badge>
                    </div>
                    <span className='text-muted-foreground text-xs'>{fmtTime(c.created_at)}</span>
                  </div>

                  {/* Body — render HTML if available, else raw markdown */}
                  {(c as { body_html?: string }).body_html ? (
                    <div
                      className='md-body prose prose-invert prose-sm max-w-none'
                      dangerouslySetInnerHTML={{ __html: (c as { body_html?: string }).body_html! }}
                    />
                  ) : c.body ? (
                    <pre className='font-mono text-xs whitespace-pre-wrap break-words'>{c.body}</pre>
                  ) : null}

                  <CommentAttachments commentId={c.id} />
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
