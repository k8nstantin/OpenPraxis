import { useMemo, useRef, useState } from 'react'
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
import {
  BlockNoteComposer,
  type BlockNoteComposerHandle,
} from '@/components/blocknote-composer'
import { BlockNoteReadView } from '@/components/blocknote-read-view'
import { claimAttachment } from '@/lib/queries/attachments'

const TYPE_LABEL: Record<string, string> = {
  prompt:  'Prompt',
  comment: 'Comment',
}

const COMPOSE_TYPES = [
  'comment',
  'prompt',
] as const

function fmtTime(v: string | number | undefined): string {
  if (!v) return ''
  try {
    // created_at is stored as unix seconds — multiply by 1000 for JS Date
    const ms = typeof v === 'number' ? v * 1000 : Date.parse(v)
    return new Date(ms).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  } catch { return String(v) }
}

export function CommentsTab({ kind, entityId }: { kind: EntityKind; entityId: string }) {
  const qc = useQueryClient()
  const comments = useEntityComments(kind, entityId)
  const create = useCreateEntityComment(kind, entityId)
  const update = useUpdateEntity(kind, entityId)

  const [filter, setFilter] = useState('all')
  const [composeType, setComposeType] = useState<keyof typeof TYPE_LABEL>('comment')
  const [composing, setComposing] = useState(false)
  const [posting, setPosting] = useState(false)
  const composerRef = useRef<BlockNoteComposerHandle>(null)

  const real = useMemo<Comment[]>(() => {
    if (!comments.data) return []
    return comments.data.filter(c => !(c.body ?? '').includes('<dependency_revision>'))
  }, [comments.data])

  // Only the most recent prompt gets the emerald highlight
  const latestDescId = useMemo(() => {
    const revs = real.filter(c => c.type === 'prompt')
    return revs[0]?.id ?? null
  }, [real])

  const types = useMemo(() => Array.from(new Set(real.map(c => c.type))), [real])

  const visible = useMemo<Comment[]>(() => {
    const list = filter === 'all' ? real : real.filter(c => c.type === filter)
    return list.slice().sort((a, b) => {
      const ta = typeof a.created_at === 'number' ? a.created_at : 0
      const tb = typeof b.created_at === 'number' ? b.created_at : 0
      return tb - ta
    })
  }, [real, filter])

  const close = () => { setComposing(false); composerRef.current?.clear() }

  const post = async () => {
    if (!composerRef.current) return
    const body = (await composerRef.current.getMarkdown()).trim()
    const attachmentIds = composerRef.current.getAttachmentIDs()
    if (!body && attachmentIds.length === 0) return
    setPosting(true)
    try {
      const created = await create.mutateAsync({
        author: 'operator',
        type: composeType,
        body: body || '(attachment only)',
      }) as { id?: string } | undefined

      const newId = created?.id
      if (newId && attachmentIds.length > 0) {
        await Promise.all(
          attachmentIds.map(aid => claimAttachment(aid, newId).catch(err => {
            console.error('attachment claim failed', aid, err)
          }))
        )
        qc.invalidateQueries({ queryKey: ['attachments', 'comment', newId] })
      }

      // Description-as-comment: also PATCH the entity description field
      if (composeType === 'prompt' && body) {
        update.mutate({ description: body } as Parameters<typeof update.mutate>[0])
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

      {/* BlockNote Composer */}
      {composing && (
        <Card className='gap-0 py-0'>
          <CardContent className='space-y-2 px-3 py-2'>
            <BlockNoteComposer
              ref={composerRef}
              onSave={post}
              onCancel={close}
              placeholder={
                composeType === 'prompt'
                  ? 'Write description… drop files, paste images, type / for blocks (Cmd-Enter to post)'
                  : 'Add a comment… drop files, paste images, type / for blocks (Cmd-Enter to post)'
              }
            />
            <div className='flex items-center justify-end gap-2'>
              {create.isError && (
                <span className='mr-auto text-xs text-rose-400'>Post failed: {String(create.error)}</span>
              )}
              <Select value={composeType} onValueChange={v => setComposeType(v as keyof typeof TYPE_LABEL)}>
                <SelectTrigger className='h-7 w-40 text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {COMPOSE_TYPES.map(t => (
                    <SelectItem key={t} value={t}>{TYPE_LABEL[t]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type='button' variant='ghost' size='sm' onClick={close} disabled={posting}>Cancel</Button>
              <Button type='button' size='sm' onClick={post} disabled={posting}>
                {posting
                  ? composeType === 'prompt' ? 'Saving…' : 'Posting…'
                  : composeType === 'prompt' ? 'Post as Prompt' : 'Post'}
              </Button>
            </div>
          </CardContent>
        </Card>
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
                    c.type === 'prompt' && c.id === latestDescId && 'border-l-2 border-emerald-400/60 bg-emerald-400/5',
                    c.type === 'prompt' && c.id !== latestDescId && 'opacity-50',
                  )}
                >
                  <div className='flex items-center justify-between gap-2'>
                    <div className='flex items-center gap-2'>
                      <code className='font-mono text-[11px]'>{c.author.slice(0, 16)}</code>
                      <Badge
                        variant={c.type === 'prompt' ? 'secondary' : 'outline'}
                        className='text-[10px]'
                      >
                        {TYPE_LABEL[c.type] ?? c.type}
                      </Badge>
                    </div>
                    <span className='text-muted-foreground text-xs'>{fmtTime(c.created_at)}</span>
                  </div>

                  <BlockNoteReadView markdown={c.body ?? ''} />
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
