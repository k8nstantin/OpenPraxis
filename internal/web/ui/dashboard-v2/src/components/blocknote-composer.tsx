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

// BlockNote-based markdown composer. Default schema for now — the
// custom mention + entity-card schema raised a ProseMirror init
// error in the browser ("Cannot read properties of undefined
// (reading 'node')"). Restoring the rich features needs a fresh
// schema build that's actually browser-tested before we ship it.
//
// Wired:
//   - drag-drop + paste-image upload via uploadFile → orphan attachment
//   - default toolbar, formatting, code blocks, tables, task lists,
//     slash menu, side menu, keyboard shortcuts (Cmd+B/I/U/E/K/Z +
//     our Cmd-Enter / Esc bridge)
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
  const attachmentIdsRef = useRef<string[]>([])

  const uploadFile = useCallback(async (file: File): Promise<string> => {
    const a = await uploadOrphanAttachment(file)
    attachmentIdsRef.current.push(a.id)
    return a.url
  }, [])

  const editor = useCreateBlockNote({ uploadFile })

  // Hydrate from markdown on mount. Editor instance is stable per
  // mount; cancel guard prevents the parse promise resolving after
  // unmount.
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
