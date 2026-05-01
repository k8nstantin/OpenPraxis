import { useEffect } from 'react'
import { BlockNoteView } from '@blocknote/mantine'
import { useCreateBlockNote } from '@blocknote/react'
import { opSchema } from './blocknote-schema'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'

// BlockNoteReadView — renders existing markdown as a non-editable
// BlockNote view. Same schema as the composer so mentions + entity
// cards render identically to compose. Intentionally hides slash /
// side menus and the formatting toolbar so it looks like a static
// document, not an empty edit surface.
//
// Mounts one editor instance per view — fine for the description +
// history surfaces (≤ a handful visible at once). For long lists
// (comments thread) prefer body_html or virtualize.
export function BlockNoteReadView({
  markdown,
  className,
}: {
  markdown: string
  className?: string
}) {
  const editor = useCreateBlockNote({ schema: opSchema })

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const md = markdown.trim()
      if (!md) {
        editor.replaceBlocks(editor.document, [
          { type: 'paragraph', content: [] },
        ])
        return
      }
      const blocks = await editor.tryParseMarkdownToBlocks(md)
      if (cancelled) return
      editor.replaceBlocks(editor.document, blocks)
    })()
    return () => {
      cancelled = true
    }
  }, [markdown, editor])

  return (
    <div className={className}>
      <BlockNoteView
        editor={editor}
        editable={false}
        theme='dark'
        slashMenu={false}
        sideMenu={false}
        formattingToolbar={false}
        linkToolbar={false}
        filePanel={false}
        tableHandles={false}
      />
    </div>
  )
}
