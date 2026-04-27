import clsx from 'clsx';

export type StatusKind =
  | 'open'
  | 'closed'
  | 'draft'
  | 'archive'
  | 'running'
  | 'waiting'
  | 'scheduled'
  | 'completed'
  | 'failed'
  | 'unknown';

export interface StatusDotProps {
  status: string;
  size?: 'sm' | 'md';
  label?: string;
  className?: string;
}

export function StatusDot({ status, size = 'sm', label, className }: StatusDotProps) {
  return (
    <span
      role={label ? 'img' : undefined}
      aria-label={label ?? status}
      title={label ?? status}
      className={clsx('ui-dot', `ui-dot--${size}`, `ui-dot--${normalize(status)}`, className)}
    />
  );
}

function normalize(status: string): StatusKind {
  const s = status.toLowerCase();
  switch (s) {
    case 'open':
    case 'closed':
    case 'draft':
    case 'archive':
    case 'running':
    case 'waiting':
    case 'scheduled':
    case 'completed':
    case 'failed':
      return s as StatusKind;
    default:
      return 'unknown';
  }
}
