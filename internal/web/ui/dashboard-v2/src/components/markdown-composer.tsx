import { useCallback, useRef, useState } from 'react'
import { Paperclip, X } from 'lucide-react'
import { MarkdownEditor } from '@/components/markdown-editor'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatBytes } from '@/lib/queries/attachments'

// Pending file the operator has picked / dropped / pasted but hasn't
// posted yet. Real upload happens after the comment is created so the
// attachment row can carry the canonical comment_id without a sentinel
// dance. Limited to 10 per compose to keep the chip bar sane.
export interface PendingAttachment {
  id: string // local-only; uuid-ish for React keys
  file: File
  preview?: string // object URL for image thumbnails
}

const MAX_PENDING = 10

function makeLocalID(): string {
  return Math.random().toString(36).slice(2, 12)
}

// MarkdownComposer wraps the existing MarkdownEditor with three things:
//   1. Drop zone that accepts dragged files (full-zone hit area; the
//      editor itself stays interactive).
//   2. Paste handler — Cmd-Shift-4 → Cmd-V uploads the screenshot.
//   3. Pending-attachments bar with thumbnails + remove buttons.
//
// Keeps the editor's keyboard shortcuts (Cmd-B/I/E/K/Enter) intact —
// MarkdownEditor still owns the textarea + toolbar, the wrapper only
// adds outer chrome.
export function MarkdownComposer({
  value,
  onChange,
  onSave,
  onCancel,
  pending,
  onAddPending,
  onRemovePending,
  placeholder,
  compact = false,
  autoFocus = false,
}: {
  value: string
  onChange: (next: string) => void
  onSave?: () => void
  onCancel?: () => void
  pending: PendingAttachment[]
  onAddPending: (files: File[]) => void
  onRemovePending: (id: string) => void
  placeholder?: string
  compact?: boolean
  autoFocus?: boolean
}) {
  const [dragActive, setDragActive] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleFiles = useCallback(
    (files: FileList | File[]) => {
      const arr = Array.from(files).slice(0, MAX_PENDING - pending.length)
      if (arr.length > 0) onAddPending(arr)
    },
    [onAddPending, pending.length]
  )

  const onDragOver = (e: React.DragEvent) => {
    if (e.dataTransfer.types.includes('Files')) {
      e.preventDefault()
      setDragActive(true)
    }
  }
  const onDragLeave = (e: React.DragEvent) => {
    if (e.currentTarget === e.target) setDragActive(false)
  }
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragActive(false)
    if (e.dataTransfer.files?.length) handleFiles(e.dataTransfer.files)
  }
  // Paste — listens via React onPaste on a wrapper div. clipboardData.files
  // carries File objects only when the OS clipboard has a file/image; plain
  // text pastes fall through to the textarea unchanged.
  const onPaste = (e: React.ClipboardEvent) => {
    const files = e.clipboardData?.files
    if (files && files.length > 0) {
      e.preventDefault()
      handleFiles(files)
    }
  }
  const onPickerChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files?.length) handleFiles(e.target.files)
    // Reset so picking the same file twice in a row still fires onChange.
    e.target.value = ''
  }

  return (
    <div
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      onPaste={onPaste}
      className={cn(
        'relative flex flex-col gap-2',
        dragActive && 'ring-2 ring-emerald-400 ring-offset-2 ring-offset-background rounded-md'
      )}
    >
      {pending.length > 0 ? (
        <div className='flex flex-wrap gap-2'>
          {pending.map((p) => (
            <div
              key={p.id}
              className='border-border bg-muted/40 group relative flex items-center gap-2 rounded-sm border px-2 py-1 text-xs'
              title={`${p.file.name} — ${formatBytes(p.file.size)}`}
            >
              {p.preview ? (
                <img
                  src={p.preview}
                  alt=''
                  className='h-8 w-8 rounded-sm object-cover'
                />
              ) : (
                <span className='font-mono text-[10px]'>
                  {p.file.type || 'file'}
                </span>
              )}
              <span className='max-w-[10rem] truncate'>{p.file.name}</span>
              <span className='text-muted-foreground'>
                {formatBytes(p.file.size)}
              </span>
              <button
                type='button'
                aria-label='Remove'
                className='ml-1 opacity-50 hover:opacity-100'
                onClick={() => onRemovePending(p.id)}
              >
                <X className='h-3 w-3' />
              </button>
            </div>
          ))}
        </div>
      ) : null}

      <MarkdownEditor
        value={value}
        onChange={onChange}
        onSave={onSave}
        onCancel={onCancel}
        placeholder={placeholder}
        compact={compact}
        autoFocus={autoFocus}
      />

      <div className='flex items-center gap-2 text-xs text-muted-foreground'>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-7 px-2 text-xs'
          onClick={() => fileInputRef.current?.click()}
        >
          <Paperclip className='mr-1 h-3 w-3' />
          Attach
        </Button>
        <span>
          Drop files here or paste an image — {pending.length}/{MAX_PENDING} pending
        </span>
        <input
          ref={fileInputRef}
          type='file'
          multiple
          className='hidden'
          onChange={onPickerChange}
        />
      </div>

      {dragActive ? (
        <div className='pointer-events-none absolute inset-0 flex items-center justify-center rounded-md border-2 border-dashed border-emerald-400 bg-emerald-400/10 text-sm text-emerald-100'>
          Drop to attach
        </div>
      ) : null}
    </div>
  )
}

// usePendingAttachments — hook that owns the pending-files state and
// builds preview object URLs for image mimes. Returned helpers wire
// 1:1 into MarkdownComposer's onAddPending / onRemovePending props.
export function usePendingAttachments() {
  const [pending, setPending] = useState<PendingAttachment[]>([])

  const add = useCallback((files: File[]) => {
    setPending((prev) => [
      ...prev,
      ...files.map((f) => ({
        id: makeLocalID(),
        file: f,
        preview: f.type.startsWith('image/')
          ? URL.createObjectURL(f)
          : undefined,
      })),
    ])
  }, [])

  const remove = useCallback((id: string) => {
    setPending((prev) => {
      const next = prev.filter((p) => p.id !== id)
      const removed = prev.find((p) => p.id === id)
      if (removed?.preview) URL.revokeObjectURL(removed.preview)
      return next
    })
  }, [])

  const clear = useCallback(() => {
    setPending((prev) => {
      prev.forEach((p) => {
        if (p.preview) URL.revokeObjectURL(p.preview)
      })
      return []
    })
  }, [])

  return { pending, add, remove, clear }
}
