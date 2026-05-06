/**
 * ContentBlock — unified text + file attachment element for all entity types.
 *
 * Uses BlockNoteComposer which has native drag/drop/paste attachment support
 * via the uploadFile callback — no custom editor needed.
 *
 * All entity types → label="Prompt"
 * All entity types → label="Prompt"
 
 
 */
import { useRef, useState } from 'react'
import { Pencil, ChevronDown, ChevronUp } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { CommentAttachments } from '@/components/comment-attachments'
import {
  BlockNoteComposer,
  type BlockNoteComposerHandle,
} from '@/components/blocknote-composer'
import { BlockNoteReadView } from '@/components/blocknote-read-view'
import {
  useEntityComments,
  useCreateEntityComment,
  useUpdateEntity,
  type EntityKind,
} from '@/lib/queries/entity'
import { claimAttachment } from '@/lib/queries/attachments'

interface ContentBlockProps {
  entityId: string
  kind: EntityKind
  label?: string
  placeholder?: string
}

export function ContentBlock({ entityId, kind, label = 'Prompt', placeholder }: ContentBlockProps) {
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const composerRef = useRef<BlockNoteComposerHandle>(null)
  const qc = useQueryClient()

  const { data: comments, isLoading } = useEntityComments(kind, entityId)
  const createComment = useCreateEntityComment(kind, entityId)
  const updateEntity = useUpdateEntity(kind, entityId)

  const revisions = (comments ?? []).filter(c => c.type === 'prompt')
  const latest = revisions[0]

  const startEdit = () => setEditing(true)
  const cancel = () => { setEditing(false); composerRef.current?.clear() }

  const save = async () => {
    if (!composerRef.current) return
    setSaving(true)
    try {
      const body = (await composerRef.current.getMarkdown()).trim()
      const attachmentIds = composerRef.current.getAttachmentIDs()
      if (!body && attachmentIds.length === 0) return

      const created = await createComment.mutateAsync({
        author: 'operator',
        type: 'prompt',
        body: body || '(attachment only)',
      }) as { id?: string } | undefined

      const newId = created?.id
      if (newId && attachmentIds.length > 0) {
        await Promise.all(attachmentIds.map(aid => claimAttachment(aid, newId).catch(() => {})))
        qc.invalidateQueries({ queryKey: ['attachments', 'comment', newId] })
      }

      if (body) {
        updateEntity.mutate({ description: body } as Parameters<typeof updateEntity.mutate>[0])
      }

      setEditing(false)
      composerRef.current?.clear()
    } catch (e) {
      console.error(e)
    } finally {
      setSaving(false)
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
            {!editing && (
              <Button variant='ghost' size='sm' className='h-7 px-2 text-xs' onClick={startEdit}>
                <Pencil className='mr-1 h-3 w-3' />Edit
              </Button>
            )}
          </div>
          {!editing && (
            <>
              <div className='px-4 py-3'>
                <BlockNoteReadView markdown={latest.body ?? ''} />
              </div>
              <div className='px-4 pb-3'>
                <CommentAttachments commentId={latest.id} />
              </div>
            </>
          )}
        </div>
      ) : !editing ? (
        <button
          type='button'
          className='w-full rounded-lg border border-dashed p-6 text-sm text-muted-foreground hover:border-primary/40 hover:text-foreground transition-colors text-center'
          onClick={startEdit}
        >
          + Add {label.toLowerCase()}
        </button>
      ) : null}

      {/* BlockNote editor (drag/drop/paste files natively) */}
      {editing && (
        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <span className='text-xs font-medium text-muted-foreground'>
              {latest ? `Update ${label}` : `Add ${label}`}
            </span>
            <Button variant='ghost' size='sm' className='h-6 px-2 text-xs' onClick={cancel} disabled={saving}>
              Cancel
            </Button>
          </div>
          <BlockNoteComposer
            ref={composerRef}
            initialMarkdown={latest?.body ?? ''}
            onSave={save}
            onCancel={cancel}
            placeholder={placeholder ?? `Write ${label.toLowerCase()} here… drop files, paste images, type / for blocks`}
          />
          <div className='flex items-center justify-end gap-2'>
            {updateEntity.isError && (
              <span className='mr-auto text-xs text-rose-400'>Save failed</span>
            )}
            <Button variant='ghost' size='sm' onClick={cancel} disabled={saving}>Cancel</Button>
            <Button size='sm' onClick={save} disabled={saving}>
              {saving ? 'Saving…' : `Save ${label}`}
            </Button>
          </div>
        </div>
      )}

      {!editing && !latest && (
        <Button variant='outline' size='sm' className='text-xs' onClick={startEdit}>
          <Pencil className='mr-1 h-3 w-3' />Add {label}
        </Button>
      )}

      {/* Revision history (collapsible) */}
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
                <div key={rev.id} className='rounded border border-dashed p-3 opacity-60'>
                  <div className='mb-1 font-mono text-[10px] text-muted-foreground'>{rev.created_at}</div>
                  <BlockNoteReadView markdown={rev.body ?? ''} />
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
