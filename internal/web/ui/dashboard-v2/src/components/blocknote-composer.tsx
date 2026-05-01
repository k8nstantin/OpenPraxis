import { useCallback, useEffect, useImperativeHandle, useRef } from 'react'
import { filterSuggestionItems } from '@blocknote/core'
import { BlockNoteView } from '@blocknote/mantine'
import {
  SuggestionMenuController,
  getDefaultReactSlashMenuItems,
  useCreateBlockNote,
  type DefaultReactSuggestionItem,
  type BlockNoteEditor,
} from '@blocknote/react'
import {
  Bold,
  Code,
  Heading1,
  Heading2,
  Heading3,
  Italic,
  Link as LinkIcon,
  List,
  ListChecks,
  ListOrdered,
  Quote,
  Strikethrough,
  Underline,
  Layers,
  Package,
} from 'lucide-react'
import { searchEntities, type EntityHit } from '@/lib/queries/entity-search'
import { uploadOrphanAttachment } from '@/lib/queries/attachments'
import { opSchema } from './blocknote-schema'
import { cn } from '@/lib/utils'
import '@blocknote/core/fonts/inter.css'
import '@blocknote/mantine/style.css'

export type BlockNoteComposerHandle = {
  getMarkdown: () => Promise<string>
  getAttachmentIDs: () => string[]
  clear: () => void
}

// Permanent formatting toolbar. Each button reaches into editor.*
// directly — FormattingToolbarController is selection-driven and
// won't render without a text selection. These show always.
function ToolbarBtn({
  title,
  onClick,
  children,
}: {
  title: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type='button'
      tabIndex={-1}
      title={title}
      onMouseDown={(e) => {
        e.preventDefault() // keep editor focused
        onClick()
      }}
      className='text-muted-foreground hover:text-foreground hover:bg-accent rounded-sm p-1 transition-colors'
    >
      {children}
    </button>
  )
}

function Sep() {
  return <span className='bg-border mx-0.5 inline-block h-4 w-px' />
}

function Toolbar({ editor }: { editor: BlockNoteEditor }) {
  const block = () => editor.getTextCursorPosition().block

  const setType = (type: string, props?: Record<string, unknown>) =>
    editor.updateBlock(block(), { type, ...(props && { props }) } as never)

  const toggle = (style: 'bold' | 'italic' | 'underline' | 'strike' | 'code') =>
    editor.toggleStyles({ [style]: true } as never)

  const insertLink = () => {
    const url = window.prompt('URL') ?? ''
    if (url.trim()) editor.createLink(url.trim())
  }

  const iconCls = 'h-3.5 w-3.5'
  return (
    <div className='border-border bg-muted/30 flex flex-wrap items-center gap-0.5 border-b px-2 py-1'>
      <ToolbarBtn title='Heading 1' onClick={() => setType('heading', { level: 1 })}>
        <Heading1 className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Heading 2' onClick={() => setType('heading', { level: 2 })}>
        <Heading2 className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Heading 3' onClick={() => setType('heading', { level: 3 })}>
        <Heading3 className={iconCls} />
      </ToolbarBtn>
      <Sep />
      <ToolbarBtn title='Bold (⌘B)' onClick={() => toggle('bold')}>
        <Bold className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Italic (⌘I)' onClick={() => toggle('italic')}>
        <Italic className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Underline (⌘U)' onClick={() => toggle('underline')}>
        <Underline className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Strikethrough' onClick={() => toggle('strike')}>
        <Strikethrough className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Inline code (⌘E)' onClick={() => toggle('code')}>
        <Code className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Link (⌘K)' onClick={insertLink}>
        <LinkIcon className={iconCls} />
      </ToolbarBtn>
      <Sep />
      <ToolbarBtn title='Bullet list' onClick={() => setType('bulletListItem')}>
        <List className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Numbered list' onClick={() => setType('numberedListItem')}>
        <ListOrdered className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Task list' onClick={() => setType('checkListItem')}>
        <ListChecks className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Quote' onClick={() => setType('quote')}>
        <Quote className={iconCls} />
      </ToolbarBtn>
      <ToolbarBtn title='Code block' onClick={() => setType('codeBlock')}>
        <Code className={cn(iconCls, 'opacity-70')} />
      </ToolbarBtn>
    </div>
  )
}

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
    dictionary: {
      placeholders: {
        default: placeholder ?? "Type '/' for blocks, '@' to mention…",
        emptyDocument: undefined,
      },
    } as never,
  })

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
    return () => { cancelled = true }
  }, [editor, initialMarkdown])

  useImperativeHandle(ref, () => ({
    getMarkdown: () => editor.blocksToMarkdownLossy(editor.document),
    getAttachmentIDs: () => attachmentIdsRef.current.slice(),
    clear: () => {
      editor.replaceBlocks(editor.document, [{ type: 'paragraph', content: [] }])
      attachmentIdsRef.current = []
    },
  }), [editor])

  const onKeyDownCapture = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault(); e.stopPropagation(); onSave?.()
    } else if (e.key === 'Escape') {
      e.preventDefault(); e.stopPropagation(); onCancel?.()
    }
  }

  const getMentionItems = useCallback(
    async (query: string): Promise<DefaultReactSuggestionItem[]> => {
      const hits = await searchEntities(query)
      return hits.map((hit: EntityHit) => ({
        title: hit.title,
        subtext: `${hit.kind} · ${hit.id.slice(0, 8)}`,
        onItemClick: () =>
          editor.insertInlineContent([
            { type: 'mention', props: { kind: hit.kind, id: hit.id, label: hit.title } },
            ' ',
          ]),
        aliases: [hit.id, hit.id.slice(0, 8)],
        group: hit.kind,
      }))
    },
    [editor]
  )

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
          const id = (window.prompt(`${label} id (UUID)`) ?? '').trim()
          if (!id) return
          editor.insertBlocks(
            [{ type, props: { id } }],
            editor.getTextCursorPosition().block,
            'after'
          )
        },
      })
      return filterSuggestionItems(
        [
          insertCard('productCard', 'Product card', Package),
          insertCard('manifestCard', 'Manifest card', Layers),
          insertCard('taskCard', 'Task card', ListChecks),
          ...defaults,
        ],
        query
      )
    },
    [editor]
  )

  return (
    <div
      className='border-border bg-card overflow-hidden rounded-md border'
      onKeyDownCapture={onKeyDownCapture}
    >
      <Toolbar editor={editor as BlockNoteEditor} />
      <BlockNoteView editor={editor} theme='dark' slashMenu={false}>
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
