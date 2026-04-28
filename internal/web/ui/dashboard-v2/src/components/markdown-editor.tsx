import { useEffect, useId, useRef } from 'react'
// Side-effect import: registers the <markdown-toolbar> custom element
// globally on first import. Same web component Portal A uses (the H /
// B / I / " / <> / 🔗 / • / 1. / ☐ toolbar in the legacy edit view).
import '@github/markdown-toolbar-element'
import { cn } from '@/lib/utils'

// Toolbar action set — verbatim from Portal A's components/editor.js so
// operators see identical buttons in identical order across both
// portals. One toolbar, used everywhere markdown is composed.
const ACTIONS: Array<
  | { tag: string; title: string; glyph: string; bold?: boolean; italic?: boolean }
  | '|'
> = [
  { tag: 'md-header', title: 'Heading', glyph: 'H' },
  { tag: 'md-bold', title: 'Bold (Cmd+B)', glyph: 'B', bold: true },
  { tag: 'md-italic', title: 'Italic (Cmd+I)', glyph: 'I', italic: true },
  '|',
  { tag: 'md-quote', title: 'Quote', glyph: '❝' },
  { tag: 'md-code', title: 'Code (Cmd+E)', glyph: '<>' },
  { tag: 'md-link', title: 'Link (Cmd+K)', glyph: '🔗' },
  '|',
  { tag: 'md-unordered-list', title: 'Bullet list', glyph: '• —' },
  { tag: 'md-ordered-list', title: 'Numbered list', glyph: '1.' },
  { tag: 'md-task-list', title: 'Task list', glyph: '☐' },
]

// Required so TypeScript accepts the <markdown-toolbar> custom element +
// the <md-bold>, <md-header> etc. inner buttons in JSX without typing
// each one individually. The web component is registered globally by
// the side-effect import above.
declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'markdown-toolbar': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement> & { for?: string },
        HTMLElement
      >
      'md-header': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-bold': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-italic': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-quote': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-code': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-link': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-unordered-list': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-ordered-list': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
      'md-task-list': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >
    }
  }
}

// MarkdownEditor — React wrapper around <markdown-toolbar> + <textarea>.
// Mirrors OL.mountEditor's API: controlled value, onSave (Cmd-Enter),
// onCancel (Escape). Same buttons, same shortcuts, same visual rhythm
// as Portal A.
export function MarkdownEditor({
  value,
  onChange,
  onSave,
  onCancel,
  placeholder,
  compact = false,
  autoFocus = false,
}: {
  value: string
  onChange: (next: string) => void
  onSave?: () => void
  onCancel?: () => void
  placeholder?: string
  compact?: boolean
  autoFocus?: boolean
}) {
  const reactId = useId()
  // The markdown-toolbar element looks up its target by `for=<id>` —
  // useId guarantees uniqueness across multiple mounted editors on the
  // same page (e.g. Description edit + Comments compose simultaneously).
  const textareaId = `md-editor-${reactId.replace(/:/g, '-')}`
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (autoFocus) textareaRef.current?.focus()
  }, [autoFocus])

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      if (onSave) {
        e.preventDefault()
        onSave()
      }
    } else if (e.key === 'Escape') {
      if (onCancel) {
        e.preventDefault()
        onCancel()
      }
    }
  }

  return (
    <div className='flex flex-col overflow-hidden rounded-md border bg-input/30'>
      <markdown-toolbar
        for={textareaId}
        className='border-border bg-muted/40 flex items-center gap-0.5 border-b px-2 py-1.5'
      >
        {ACTIONS.map((a, i) => {
          if (a === '|') {
            return (
              <span
                key={`sep-${i}`}
                className='bg-border mx-1 inline-block h-4 w-px'
              />
            )
          }
          const Tag = a.tag as keyof React.JSX.IntrinsicElements
          return (
            // @ts-expect-error — dynamic custom-element tag name
            <Tag key={a.tag}>
              <button
                type='button'
                tabIndex={-1}
                title={a.title}
                className={cn(
                  'text-muted-foreground hover:bg-input hover:text-foreground rounded-sm px-2 py-1 font-mono text-xs',
                  a.bold && 'font-bold',
                  a.italic && 'italic'
                )}
              >
                {a.glyph}
              </button>
            </Tag>
          )
        })}
      </markdown-toolbar>
      <textarea
        id={textareaId}
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        className={cn(
          'bg-input/30 text-foreground w-full resize-y border-0 px-3 py-2 font-mono text-sm leading-relaxed outline-none',
          compact ? 'min-h-24' : 'min-h-60'
        )}
      />
    </div>
  )
}
