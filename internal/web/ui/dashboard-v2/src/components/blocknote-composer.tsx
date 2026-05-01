import { useCallback, useEffect, useImperativeHandle, useRef } from 'react'
import { filterSuggestionItems } from '@blocknote/core'
import { BlockNoteView } from '@blocknote/mantine'
import {
  SuggestionMenuController,
  getDefaultReactSlashMenuItems,
  useCreateBlockNote,
  type DefaultReactSuggestionItem,
} from '@blocknote/react'
import { Layers, ListChecks, Package } from 'lucide-react'
import {
  searchEntities,
  type EntityHit,
} from '@/lib/queries/entity-search'
import { uploadOrphanAttachment } from '@/lib/queries/attachments'
import { opSchema } from './blocknote-schema'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'

export type BlockNoteComposerHandle = {
  getMarkdown: () => Promise<string>
  getAttachmentIDs: () => string[]
  clear: () => void
}

// BlockNote-based markdown composer. Replaces the textarea + toolbar
// editor on every compose surface. The editor is wired with:
//   - drag-drop + paste-image upload via uploadFile → orphan attachment
//   - @-mentions for products / manifests / tasks via SuggestionMenuController
//   - extended slash menu with 3 custom items for embedding entity cards
//   - default toolbar, formatting, code blocks, tables, task lists,
//     keyboard shortcuts (Cmd+B/I/U/E/K/Z + our Cmd-Enter / Esc bridge)
//
// Markdown round-trip: mentions degrade to plain anchor tags inside
// the body (`[@label](href)`); custom card blocks degrade to anchor
// links. They re-rehydrate on fresh inserts but persist as plain
// links across save/load, matching the markdown source-of-truth.
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

  const editor = useCreateBlockNote({
    schema: opSchema,
    uploadFile,
  })

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

  // @-mention items — fan-out search across products, manifests, tasks.
  const getMentionItems = useCallback(
    async (query: string): Promise<DefaultReactSuggestionItem[]> => {
      const hits = await searchEntities(query)
      return hits.map((hit: EntityHit) => ({
        title: hit.title,
        subtext: `${hit.kind} · ${hit.id.slice(0, 8)}`,
        onItemClick: () => {
          editor.insertInlineContent([
            {
              type: 'mention',
              props: { kind: hit.kind, id: hit.id, label: hit.title },
            },
            ' ',
          ])
        },
        aliases: [hit.id, hit.id.slice(0, 8)],
        group: hit.kind,
      }))
    },
    [editor]
  )

  // Slash menu — defaults plus three "Embed … card" items that prompt
  // the operator for an entity id, then insert the corresponding
  // custom block.
  const getSlashItems = useCallback(
    async (query: string): Promise<DefaultReactSuggestionItem[]> => {
      const defaults = getDefaultReactSlashMenuItems(editor)
      const insertCard = (
        type: 'productCard' | 'manifestCard' | 'taskCard',
        label: string,
        Icon: typeof Layers
      ): DefaultReactSuggestionItem => ({
        title: label,
        subtext: `Embed an OpenPraxis ${label.toLowerCase()} by id`,
        aliases: ['embed', 'card', label.toLowerCase()],
        group: 'OpenPraxis',
        icon: <Icon className='h-4 w-4' />,
        onItemClick: () => {
          const id = window.prompt(`${label} id (UUID)`) ?? ''
          const trimmed = id.trim()
          if (!trimmed) return
          editor.insertBlocks(
            [{ type, props: { id: trimmed } }],
            editor.getTextCursorPosition().block,
            'after'
          )
        },
      })
      const custom: DefaultReactSuggestionItem[] = [
        insertCard('productCard', 'Product card', Package),
        insertCard('manifestCard', 'Manifest card', Layers),
        insertCard('taskCard', 'Task card', ListChecks),
      ]
      return filterSuggestionItems([...custom, ...defaults], query)
    },
    [editor]
  )

  return (
    <div
      className='border-border bg-input/30 overflow-hidden rounded-md border'
      onKeyDownCapture={onKeyDownCapture}
      data-placeholder={placeholder}
    >
      <BlockNoteView
        editor={editor}
        theme='dark'
        slashMenu={false}
      >
        <SuggestionMenuController
          triggerCharacter='@'
          minQueryLength={1}
          getItems={getMentionItems}
        />
        <SuggestionMenuController
          triggerCharacter='/'
          getItems={getSlashItems}
        />
      </BlockNoteView>
    </div>
  )
}
