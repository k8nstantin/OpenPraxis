// Render an entity description honouring the DV/M5 trust rule (PR #237):
// when the server pre-rendered HTML, render it verbatim; otherwise pass
// raw text through react-markdown. The DescToggle store flips between
// the rendered view and the literal markup source.
import ReactMarkdown from 'react-markdown';
import { useUiStore } from '@/lib/stores/ui';

export interface MarkdownRendererProps {
  source?: string;
  html?: string;
  className?: string;
  /** Force a specific mode and ignore the global toggle (e.g. previews). */
  forceMode?: 'markup' | 'rendered';
}

export function MarkdownRenderer({ source, html, className, forceMode }: MarkdownRendererProps) {
  const storeMode = useUiStore((s) => s.descMode);
  const mode = forceMode ?? storeMode;
  const empty = !source && !html;

  if (empty) {
    return <p className={className ?? 'markdown-renderer'}><em>No description.</em></p>;
  }

  if (mode === 'markup') {
    return <pre className={(className ?? 'markdown-renderer') + ' markdown-renderer--markup'}>{source ?? html ?? ''}</pre>;
  }

  if (html) {
    return (
      <div
        className={className ?? 'markdown-renderer'}
        // DV/M5: the server already trusted the source. Reproducing the
        // legacy verbatim behavior is required for parity.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    );
  }

  return <ReactMarkdown className={className ?? 'markdown-renderer'}>{source ?? ''}</ReactMarkdown>;
}
