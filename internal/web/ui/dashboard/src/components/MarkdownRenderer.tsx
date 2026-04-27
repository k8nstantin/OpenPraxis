import ReactMarkdown from 'react-markdown';
import clsx from 'clsx';

export interface MarkdownRendererProps {
  source: string;
  className?: string;
  /** When true, raw HTML in the source is passed through verbatim (DV/M5 trust passthrough). */
  trustHtml?: boolean;
}

// Markdown surface for entity descriptions. Mirrors the legacy `md-body`
// class so styles already in styles.css continue to apply. Raw HTML
// passthrough matches the DV/M5 contract: the dashboard renders agent-
// authored XML/HTML verbatim because the agent is the trusted author.
export function MarkdownRenderer({ source, className, trustHtml = true }: MarkdownRendererProps) {
  if (trustHtml) {
    return (
      <div
        className={clsx('md-body', className)}
        // The author surface is trusted (DV/M5). External user input must
        // never reach this component — wrap untrusted strings with the
        // sanitised renderer instead.
        dangerouslySetInnerHTML={{ __html: source }}
      />
    );
  }
  return (
    <div className={clsx('md-body', className)}>
      <ReactMarkdown>{source}</ReactMarkdown>
    </div>
  );
}
