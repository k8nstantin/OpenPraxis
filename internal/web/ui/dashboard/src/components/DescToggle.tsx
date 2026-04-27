import clsx from 'clsx';
import { useUiStore } from '../lib/stores/ui';
import { MarkdownRenderer } from './MarkdownRenderer';

export interface DescToggleProps {
  raw: string;
  rendered?: string;
  className?: string;
  /** Override the global desc mode for a single instance. */
  modeOverride?: 'markup' | 'rendered';
}

// React port of OL.descToggle (#239). Reads/writes the global
// descMode via the Zustand store so flipping one toggle updates every
// other DescToggle on the page (matches legacy cross-toggle sync).
export function DescToggle({ raw, rendered, className, modeOverride }: DescToggleProps) {
  const mode = useUiStore((s) => modeOverride ?? s.descMode);
  const setMode = useUiStore((s) => s.setDescMode);

  return (
    <div className={clsx('desc-toggle-wrap', className)}>
      <div className="desc-toggle-controls" role="tablist" aria-label="Description display mode">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'markup'}
          className={clsx('desc-toggle-btn', mode === 'markup' && 'active')}
          onClick={() => setMode('markup')}
        >
          Markup
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'rendered'}
          className={clsx('desc-toggle-btn', mode === 'rendered' && 'active')}
          onClick={() => setMode('rendered')}
        >
          Rendered
        </button>
      </div>
      {mode === 'markup' ? (
        <pre className="desc-markup">{raw}</pre>
      ) : (
        <MarkdownRenderer source={rendered ?? raw} trustHtml />
      )}
    </div>
  );
}
