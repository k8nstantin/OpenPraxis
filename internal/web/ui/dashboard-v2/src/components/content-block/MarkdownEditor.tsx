import { useEffect, useId, useRef } from 'react'
import '@github/markdown-toolbar-element'
import { cn } from '@/lib/utils'

// Toolbar buttons in the same order Portal A uses.
const ACTIONS = [
  { tag: 'md-bold',          title: 'Bold',          glyph: 'B',  bold: true },
  { tag: 'md-italic',        title: 'Italic',        glyph: 'I',  italic: true },
  { tag: 'md-code',          title: 'Code',          glyph: '<>' },
  '|',
  { tag: 'md-link',          title: 'Link',          glyph: '🔗' },
  { tag: 'md-unordered-list',title: 'Bullet list',   glyph: '•' },
  { tag: 'md-ordered-list',  title: 'Numbered list', glyph: '1.' },
  { tag: 'md-task-list',     title: 'Task list',     glyph: '☐' },
] as const

interface MarkdownEditorProps {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  minRows?: number
  onSubmit?: () => void
  className?: string
}

export function MarkdownEditor({ value, onChange, placeholder, minRows = 4, onSubmit, className }: MarkdownEditorProps) {
  const id = useId()
  const ref = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault()
        onSubmit?.()
      }
    }
    el.addEventListener('keydown', onKeyDown)
    return () => el.removeEventListener('keydown', onKeyDown)
  }, [onSubmit])

  return (
    <div className={cn('rounded-md border bg-muted/20', className)}>
      {/* @ts-expect-error — custom element */}
      <markdown-toolbar for={id} className='flex flex-wrap gap-0.5 border-b px-2 py-1'>
        {ACTIONS.map((a, i) =>
          a === '|'
            ? <span key={i} className='mx-1 text-border select-none'>|</span>
            : (
              <button
                // @ts-expect-error — custom element child
                is={a.tag}
                key={a.tag}
                title={a.title}
                type='button'
                className={cn(
                  'rounded px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground transition-colors',
                  ('bold' in a && a.bold) && 'font-bold',
                  ('italic' in a && a.italic) && 'italic',
                )}
              >
                {a.glyph}
              </button>
            )
        )}
        {/* @ts-expect-error */}
      </markdown-toolbar>
      <textarea
        id={id}
        ref={ref}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        rows={minRows}
        className='w-full resize-none bg-transparent px-3 py-2 text-sm outline-none placeholder:text-muted-foreground'
      />
    </div>
  )
}
