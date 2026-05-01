import { useCallback, useEffect, useImperativeHandle, useRef } from 'react'
import { BlockNoteView } from '@blocknote/mantine'
import { useCreateBlockNote } from '@blocknote/react'
import { uploadOrphanAttachment } from '@/lib/queries/attachments'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'

export type BlockNoteComposerHandle = {
  getMarkdown: () => Promise<string>
  getAttachmentIDs: () => string[]
  clear: () => void
}

// BlockNote-based markdown composer. Replaces the textarea + toolbar
// editor on comment compose. Drag-drop + paste-image work out of the
// box via BlockNote's image / file blocks; uploadFile streams each
// file to /api/attachments (orphan upload) so it renders inline while
// the user is still typing. The parent Claims those orphans against
// the new comment id after the post lands.
//
// Cmd-Enter + Escape are bridged via a wrapper keydown listener — the
// BlockNote editor doesn't expose first-class hooks for those.
export function BlockNoteComposer({
  initialMarkdown = '',
  onSave,
  onCancel,
  placeholder,
  ref,
}: {
  initialMarkdown?: string
  onSave?: () => void
  onCancel?: () => void
  placeholder?: string
  ref?: React.Ref<BlockNoteComposerHandle>
}) {
  // Track every orphan attachment id the editor has uploaded during
  // this compose session. Parent reads via getAttachmentIDs() on post
  // and Claims each row against the new comment_id.
  const attachmentIdsRef = useRef<string[]>([])

  const uploadFile = useCallback(async (file: File): Promise<string> => {
    const a = await uploadOrphanAttachment(file)
    attachmentIdsRef.current.push(a.id)
    return a.url
  }, [])

  const editor = useCreateBlockNote({
    uploadFile,
    initialContent: undefined, // hydrated below from initialMarkdown
  })

  // Hydrate from existing markdown (edit-existing-comment path). Empty
  // string skips. Run once when the editor instance is ready.
  const hydratedRef = useRef(false)
  useEffect(() => {
    if (hydratedRef.current) return
    if (!initialMarkdown.trim()) {
      hydratedRef.current = true
      return
    }
    let cancelled = false
    void (async () => {
      const blocks = await editor.tryParseMarkdownToBlocks(initialMarkdown)
      if (cancelled) return
      editor.replaceBlocks(editor.document, blocks)
      hydratedRef.current = true
    })()
    return () => {
      cancelled = true
    }
  }, [editor, initialMarkdown])

  useImperativeHandle(
    ref,
    () => ({
      getMarkdown: () => editor.blocksToMarkdownLossy(editor.document),
      getAttachmentIDs: () => attachmentIdsRef.current.slice(),
      clear: () => {
        editor.replaceBlocks(editor.document, [
          { type: 'paragraph', content: [] },
        ])
        attachmentIdsRef.current = []
      },
    }),
    [editor]
  )

  // Cmd/Ctrl-Enter → save, Escape → cancel. Capture phase so the
  // editor doesn't swallow them in code blocks etc.
  const onKeyDownCapture = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      if (onSave) {
        e.preventDefault()
        e.stopPropagation()
        onSave()
      }
    } else if (e.key === 'Escape') {
      if (onCancel) {
        e.preventDefault()
        e.stopPropagation()
        onCancel()
      }
    }
  }

  return (
    <div
      className='border-border bg-input/30 overflow-hidden rounded-md border'
      onKeyDownCapture={onKeyDownCapture}
      data-placeholder={placeholder}
    >
      <BlockNoteView editor={editor} theme='dark' />
    </div>
  )
}
