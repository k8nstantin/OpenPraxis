/**
 * ContentBlock — the single reusable text+attachment element used across all entity types.
 *
 * Products     → label="Description"
 * Manifests    → label="Declaration"
 * Tasks        → label="Instructions"
 * Skills/Ideas → label="Description"
 *
 * Stores content as a `description_revision` comment row (audit trail included).
 * Supports file attachments via drag/drop/paste.
 */
import { useState } from 'react'
import { Pencil, ChevronDown, ChevronUp } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { CommentAttachments } from '@/components/comment-attachments'
import { AttachmentComposer, usePendingFiles } from './AttachmentComposer'
import {
  useEntityComments,
  useCreateEntityComment,
  useUpdateEntity,
  type EntityKind,
} from '@/lib/queries/entity'
import { uploadAttachment } from '@/lib/queries/attachments'
import { attachmentKeys } from '@/lib/queries/attachments'

interface ContentBlockProps {
  entityId: string
  kind: EntityKind
  label?: string
  placeholder?: string
}

export function ContentBlock({ entityId, kind, label = 'Description', placeholder }: ContentBlockProps) {
  const [editing, setEditing] = useState(false)
  const [body, setBody] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const files = usePendingFiles()
  const qc = useQueryClient()

  const { data: comments, isLoading } = useEntityComments(kind, entityId)
  const createComment = useCreateEntityComment(kind, entityId)
  const updateEntity = useUpdateEntity(kind, entityId)

  // Latest description_revision is the "current" content
  const revisions = (comments ?? []).filter(c => c.type === 'description_revision')
  const latest = revisions[0]  // newest first from API

  async function submit() {
    const text = body.trim()
    if (!text && files.pending.length === 0) return
    setSubmitting(true)
    try {
      const created = await createComment.mutateAsync({
        type: 'description_revision',
        body: text || '(attachment only)',
        author: 'calexander',
      })
      // Upload pending attachments and claim them
      for (const p of files.pending) {
        try {
          const aid = await uploadAttachment(p.file)
          await fetch(`/api/attachments/${aid}/claim`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ comment_id: created.id }),
          })
        } catch { /* non-fatal */ }
      }
      qc.invalidateQueries({ queryKey: attachmentKeys.byComment(created.id) })
      // Also PATCH the entity description field so history endpoint reflects it
      if (text) {
        updateEntity.mutate({ description: text } as Parameters<typeof updateEntity.mutate>[0])
      }
      setBody('')
      files.clear()
      setEditing(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className='space-y-3'>
      {/* Current content */}
      {isLoading ? (
        <Skeleton className='h-20 w-full' />
      ) : latest ? (
        <div className='rounded-lg border bg-card'>
          <div className='flex items-center justify-between px-4 py-2 border-b'>
            <span className='text-xs font-medium text-muted-foreground uppercase tracking-wide'>{label}</span>
            <Button variant='ghost' size='sm' className='h-7 px-2 text-xs' onClick={() => { setBody(latest.body ?? ''); setEditing(true) }}>
              <Pencil className='mr-1 h-3 w-3' />Edit
            </Button>
          </div>
          <div
            className='md-body px-4 py-3 text-sm prose prose-invert max-w-none prose-sm'
            dangerouslySetInnerHTML={{ __html: (latest as { body_html?: string }).body_html ?? latest.body ?? '' }}
          />
          {/* Attachments on latest revision */}
          <div className='px-4 pb-3'>
            <CommentAttachments commentId={latest.id} />
          </div>
        </div>
      ) : !editing ? (
        <button
          type='button'
          className='w-full rounded-lg border border-dashed p-6 text-sm text-muted-foreground hover:border-primary/40 hover:text-foreground transition-colors text-center'
          onClick={() => setEditing(true)}
        >
          + Add {label.toLowerCase()}
        </button>
      ) : null}

      {/* Editor */}
      {editing && (
        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <span className='text-xs font-medium text-muted-foreground'>
              {latest ? `Update ${label}` : `Add ${label}`}
            </span>
            <Button variant='ghost' size='sm' className='h-6 px-2 text-xs' onClick={() => { setEditing(false); setBody('') }}>
              Cancel
            </Button>
          </div>
          <AttachmentComposer
            body={body}
            onBodyChange={setBody}
            pending={files.pending}
            onAdd={files.add}
            onRemove={files.remove}
            onSubmit={submit}
            placeholder={placeholder ?? `Write ${label.toLowerCase()} here… (Markdown supported, drag/paste files to attach)`}
            submitLabel={latest ? `Update ${label}` : `Add ${label}`}
            submitting={submitting}
          />
        </div>
      )}

      {!editing && !latest && (
        <Button variant='outline' size='sm' className='text-xs' onClick={() => setEditing(true)}>
          <Pencil className='mr-1 h-3 w-3' />Add {label}
        </Button>
      )}

      {/* History (collapsible) */}
      {revisions.length > 1 && (
        <div>
          <button
            type='button'
            className='flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors'
            onClick={() => setHistoryOpen(v => !v)}
          >
            {historyOpen ? <ChevronUp className='h-3 w-3' /> : <ChevronDown className='h-3 w-3' />}
            {revisions.length - 1} previous {revisions.length - 1 === 1 ? 'revision' : 'revisions'}
          </button>
          {historyOpen && (
            <div className='mt-2 space-y-2'>
              {revisions.slice(1).map(rev => (
                <div key={rev.id} className='rounded border border-dashed p-3 text-xs text-muted-foreground opacity-70'>
                  <div className='mb-1 font-mono text-[10px]'>{rev.created_at}</div>
                  <div
                    className='md-body prose prose-invert max-w-none prose-xs'
                    dangerouslySetInnerHTML={{ __html: (rev as { body_html?: string }).body_html ?? rev.body ?? '' }}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
