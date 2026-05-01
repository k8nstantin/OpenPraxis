import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const STORAGE_KEY = 'descMode'

type Mode = 'markup' | 'rendered'

function readMode(): Mode {
  // Default to 'rendered' so markdown content (description, comment
  // bodies, watcher findings) renders out of the box. Operator can
  // toggle to 'markup' for raw-source view; the choice persists in
  // localStorage and broadcasts via `desc-mode-change`.
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === 'markup' ? 'markup' : 'rendered'
  } catch {
    return 'rendered'
  }
}
function writeMode(m: Mode): void {
  try {
    localStorage.setItem(STORAGE_KEY, m)
  } catch {
    /* ignore */
  }
}

// DescriptionView — toggle Markup ↔ Rendered for any description /
// content body. Same UX as Portal A's OL.descToggle:
//   - default Markup (raw — what the agent receives)
//   - Rendered shows server-rendered HTML (description_html / body_html)
//   - mode persists in localStorage so future loads + other instances
//     on the page reflect the operator's preference
//
// The two-button strip matches Portal A's visual; clicking either
// updates the local state AND broadcasts to all DescriptionView
// instances on the page via a custom event so the whole dashboard
// flips together.
export function DescriptionView({
  raw,
  rendered,
  className,
  emptyLabel = 'No description set.',
}: {
  raw: string | undefined
  rendered: string | undefined
  className?: string
  emptyLabel?: string
}) {
  const [mode, setModeState] = useState<Mode>(readMode)

  useEffect(() => {
    const onChange = (e: Event) => {
      const next = (e as CustomEvent<Mode>).detail
      if (next === 'markup' || next === 'rendered') setModeState(next)
    }
    window.addEventListener('desc-mode-change', onChange as EventListener)
    return () =>
      window.removeEventListener(
        'desc-mode-change',
        onChange as EventListener
      )
  }, [])

  const setMode = (m: Mode) => {
    setModeState(m)
    writeMode(m)
    window.dispatchEvent(new CustomEvent('desc-mode-change', { detail: m }))
  }

  const isEmpty = !raw && !rendered
  const r = raw ?? ''
  const html = rendered ?? ''

  return (
    <div className={cn('w-full', className)}>
      <div className='mb-2 flex justify-end gap-1'>
        <Button
          type='button'
          variant={mode === 'markup' ? 'secondary' : 'ghost'}
          size='sm'
          className='h-6 px-2 text-xs'
          onClick={() => setMode('markup')}
        >
          Markup
        </Button>
        <Button
          type='button'
          variant={mode === 'rendered' ? 'secondary' : 'ghost'}
          size='sm'
          className='h-6 px-2 text-xs'
          onClick={() => setMode('rendered')}
        >
          Rendered
        </Button>
      </div>
      {isEmpty ? (
        <div className='text-muted-foreground text-sm italic'>{emptyLabel}</div>
      ) : mode === 'markup' ? (
        <pre className='font-mono text-sm whitespace-pre-wrap break-words'>
          {r}
        </pre>
      ) : (
        <div
          className='md-body text-sm'
          // Server-rendered HTML from the goldmark pipeline (Portal A
          // sanitizes via bluemonday before responding); inline render
          // is safe here because the source is the same backend.
          dangerouslySetInnerHTML={{ __html: html }}
        />
      )}
    </div>
  )
}
