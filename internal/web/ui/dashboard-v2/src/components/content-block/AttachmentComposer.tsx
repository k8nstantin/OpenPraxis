/**
 * AttachmentComposer — wraps MarkdownEditor with file drag/drop/paste support.
 * Pending files show as chips below the editor before posting.
 */
import { useCallback, useRef, useState } from 'react'
import { Paperclip, X } from 'lucide-react'
import { MarkdownEditor } from './MarkdownEditor'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface PendingFile {
  localId: string
  file: File
  preview?: string  // object URL for image preview
}

interface Props {
  body: string
  onBodyChange: (v: string) => void
  pending: PendingFile[]
  onAdd: (files: PendingFile[]) => void
  onRemove: (localId: string) => void
  onSubmit?: () => void
  placeholder?: string
  submitLabel?: string
  submitting?: boolean
  disabled?: boolean
}

function makeId() { return Math.random().toString(36).slice(2, 10) }

function filesToPending(files: File[]): PendingFile[] {
  return files.map(file => {
    const localId = makeId()
    const preview = file.type.startsWith('image/') ? URL.createObjectURL(file) : undefined
    return { localId, file, preview }
  })
}

export function AttachmentComposer({ body, onBodyChange, pending, onAdd, onRemove, onSubmit, placeholder, submitLabel = 'Post', submitting, disabled }: Props) {
  const [dragging, setDragging] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const addFiles = useCallback((files: FileList | File[]) => {
    const arr = Array.from(files)
    if (!arr.length) return
    onAdd(filesToPending(arr))
  }, [onAdd])

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragging(false)
    addFiles(e.dataTransfer.files)
  }, [addFiles])

  const onPaste = useCallback((e: React.ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files)
    if (files.length) addFiles(files)
  }, [addFiles])

  const canSubmit = !disabled && !submitting && (body.trim().length > 0 || pending.length > 0)

  return (
    <div
      className={cn('rounded-lg border transition-colors', dragging && 'border-primary/60 bg-primary/5')}
      onDragOver={e => { e.preventDefault(); setDragging(true) }}
      onDragLeave={() => setDragging(false)}
      onDrop={onDrop}
      onPaste={onPaste}
    >
      <MarkdownEditor
        value={body}
        onChange={onBodyChange}
        placeholder={placeholder ?? 'Write a comment… (drag/paste files to attach)'}
        onSubmit={canSubmit ? onSubmit : undefined}
        className='border-0 rounded-b-none'
      />

      {/* Pending attachment chips */}
      {pending.length > 0 && (
        <div className='flex flex-wrap gap-1.5 border-t px-3 py-2'>
          {pending.map(p => (
            <div key={p.localId} className='flex items-center gap-1 rounded border bg-muted px-2 py-0.5 text-xs'>
              {p.preview
                ? <img src={p.preview} className='h-4 w-4 rounded object-cover' />
                : <Paperclip className='h-3 w-3 text-muted-foreground' />
              }
              <span className='max-w-[120px] truncate'>{p.file.name}</span>
              <button type='button' onClick={() => onRemove(p.localId)} className='text-muted-foreground hover:text-foreground ml-0.5'>
                <X className='h-3 w-3' />
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Footer: attach button + submit */}
      <div className='flex items-center justify-between gap-2 border-t px-3 py-2'>
        <div className='flex items-center gap-1'>
          <input ref={fileInputRef} type='file' multiple className='hidden' onChange={e => addFiles(e.target.files ?? [])} />
          <Button type='button' variant='ghost' size='sm' className='h-7 px-2 text-xs text-muted-foreground' onClick={() => fileInputRef.current?.click()}>
            <Paperclip className='mr-1 h-3 w-3' />Attach
          </Button>
          <span className='text-[10px] text-muted-foreground hidden sm:inline'>or drag & drop · paste image</span>
        </div>
        <Button type='button' size='sm' className='h-7 px-3 text-xs' disabled={!canSubmit} onClick={onSubmit}>
          {submitting ? 'Posting…' : submitLabel}
        </Button>
      </div>
    </div>
  )
}

/** Hook to manage pending files state. */
export function usePendingFiles() {
  const [pending, setPending] = useState<PendingFile[]>([])
  return {
    pending,
    add: (files: PendingFile[]) => setPending(prev => [...prev, ...files].slice(0, 10)),
    remove: (id: string) => setPending(prev => prev.filter(p => p.localId !== id)),
    clear: () => setPending([]),
  }
}
