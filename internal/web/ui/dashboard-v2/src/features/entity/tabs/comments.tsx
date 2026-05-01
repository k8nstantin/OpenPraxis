import { useEffect, useMemo, useState } from 'react'
import {
  useCreateEntityComment,
  useEntityComments,
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
import {
  MarkdownComposer,
  usePendingAttachments,
} from '@/components/markdown-composer'
import { CommentAttachments } from '@/components/comment-attachments'
import { uploadAttachment } from '@/lib/queries/attachments'
import { useQueryClient } from '@tanstack/react-query'
import { attachmentKeys } from '@/lib/queries/attachments'

const TYPE_LABEL: Record<string, string> = {
  execution_review: 'Execution Review',
  description_revision: 'Description Revision',
  agent_note: 'Agent Note',
  user_note: 'Note',
  watcher_finding: 'Watcher Finding',
  decision: 'Decision',
  link: 'Link',
}

const COMPOSE_TYPES: Array<keyof typeof TYPE_LABEL> = [
  'user_note',
  'decision',
  'agent_note',
  // description_revision is the "post as description" path. The
  // existing entity-detail flow patches the description column
  // directly; posting via this dropdown also writes a comment row
  // with type=description_revision so the operator gets an audit
  // trail without losing the canonical column edit.
  'description_revision',
]

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}

type Mode = 'markup' | 'rendered'
function readMode(): Mode {
  // Default to 'rendered' — comment bodies are markdown that should
  // render by default. Operator can opt into 'markup' (raw source)
  // via the toggle; localStorage + the desc-mode-change event keep
  // this in sync with description-view.tsx.
  try {
    const v = localStorage.getItem('descMode')
    return v === 'markup' ? 'markup' : 'rendered'
  } catch {
    return 'rendered'
  }
}
function useDescMode(): [Mode, (m: Mode) => void] {
  const [mode, setMode] = useState<Mode>(readMode)
  useEffect(() => {
    const onChange = (e: Event) => {
      const next = (e as CustomEvent<Mode>).detail
      if (next === 'markup' || next === 'rendered') setMode(next)
    }
    window.addEventListener('desc-mode-change', onChange as EventListener)
    return () =>
      window.removeEventListener(
        'desc-mode-change',
        onChange as EventListener
      )
  }, [])
  const broadcast = (m: Mode) => {
    setMode(m)
    try {
      localStorage.setItem('descMode', m)
    } catch {
      /* ignore */
    }
    window.dispatchEvent(new CustomEvent('desc-mode-change', { detail: m }))
  }
  return [mode, broadcast]
}

// Comments tab — full thread + compose. Same MarkdownEditor used
// across description-edit. Cmd-Enter posts; Escape clears. Generic
// over /api/products/{id}/comments and /api/manifests/{id}/comments —
// same response shape, same compose payload.
export function CommentsTab({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const comments = useEntityComments(kind, entityId)
  const create = useCreateEntityComment(kind, entityId)
  const [filter, setFilter] = useState<string>('all')
  const [composeBody, setComposeBody] = useState('')
  const [composeType, setComposeType] =
    useState<keyof typeof TYPE_LABEL>('user_note')

  // Hide internal change-log records that the Dependencies tab posts
  // (agent_notes wrapped in <dependency_revision>) — they live in the
  // Dependencies > Revision history surface.
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
  const [mode, setMode] = useDescMode()
  const attachments = usePendingAttachments()
  const [uploadError, setUploadError] = useState<string | null>(null)
  const qc = useQueryClient()
  const open = () => setComposing(true)
  const close = () => {
    setComposing(false)
    setComposeBody('')
    attachments.clear()
    setUploadError(null)
  }
  const post = async () => {
    const body = composeBody.trim()
    if (!body && attachments.pending.length === 0) return
    setUploadError(null)
    try {
      const created = await create.mutateAsync({
        author: 'operator',
        type: composeType,
        body: body || '(attachment only)',
      })
      // Upload pending attachments serially. Failure on any one
      // surfaces the message and leaves the comment row intact —
      // operator can re-attach via a follow-up comment.
      for (const p of attachments.pending) {
        try {
          await uploadAttachment(created.id, p.file, 'operator')
        } catch (err) {
          setUploadError(
            err instanceof Error ? err.message : String(err)
          )
        }
      }
      qc.invalidateQueries({ queryKey: attachmentKeys.byComment(created.id) })
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
          <div className='flex items-center gap-1'>
            <Button
              type='button'
              variant={mode === 'markup' ? 'secondary' : 'ghost'}
              size='sm'
              className='h-7 px-2 text-xs'
              onClick={() => setMode('markup')}
            >
              Markup
            </Button>
            <Button
              type='button'
              variant={mode === 'rendered' ? 'secondary' : 'ghost'}
              size='sm'
              className='h-7 px-2 text-xs'
              onClick={() => setMode('rendered')}
            >
              Rendered
            </Button>
          </div>
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
            <MarkdownComposer
              value={composeBody}
              onChange={setComposeBody}
              onSave={post}
              onCancel={close}
              pending={attachments.pending}
              onAddPending={attachments.add}
              onRemovePending={attachments.remove}
              compact
              autoFocus
              placeholder='Add a comment in markdown… (Cmd-Enter to post, Esc to cancel). Drop files or paste a screenshot to attach.'
            />
            <div className='flex items-center justify-end gap-2'>
              {create.isError ? (
                <span className='mr-auto text-xs text-rose-400'>
                  Post failed: {String(create.error)}
                </span>
              ) : uploadError ? (
                <span className='mr-auto text-xs text-rose-400'>
                  Attachment failed: {uploadError}
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
                disabled={
                  create.isPending ||
                  (!composeBody.trim() && attachments.pending.length === 0)
                }
              >
                {create.isPending
                  ? 'Posting…'
                  : composeType === 'description_revision'
                    ? 'Post as Description'
                    : 'Post'}
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
              {visible.map((c) => {
                const isDesc = c.type === 'description_revision'
                return (
                  <div
                    key={c.id}
                    className={cn(
                      'space-y-1 p-3 text-sm',
                      isDesc &&
                        'border-l-2 border-emerald-400/60 bg-emerald-400/5'
                    )}
                  >
                    <div className='flex items-center justify-between gap-2'>
                      <div className='flex items-center gap-2'>
                        <code className='font-mono text-[11px]'>
                          {c.author.slice(0, 16)}
                        </code>
                        <Badge
                          variant={isDesc ? 'default' : 'outline'}
                          className='text-[10px]'
                        >
                          {TYPE_LABEL[c.type] ?? c.type}
                        </Badge>
                      </div>
                      <span className='text-muted-foreground text-xs'>
                        {fmtTime(c.created_at)}
                      </span>
                    </div>
                    {mode === 'rendered' &&
                    (c as { body_html?: string }).body_html ? (
                      <div
                        className='md-body text-sm'
                        dangerouslySetInnerHTML={{
                          __html:
                            (c as { body_html?: string }).body_html ?? '',
                        }}
                      />
                    ) : (
                      <pre className='font-mono text-xs whitespace-pre-wrap break-words'>
                        {c.body}
                      </pre>
                    )}
                    <CommentAttachments commentId={c.id} />
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
