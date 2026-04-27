import clsx from 'clsx';

export type StatusTone =
  | 'idle'
  | 'waiting'
  | 'scheduled'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'paused'
  | 'open'
  | 'closed';

export interface StatusDotProps {
  status: string;
  className?: string;
  /** Provide a label so screen readers announce the state. Defaults to the status string. */
  label?: string;
}

export function StatusDot({ status, className, label }: StatusDotProps) {
  return (
    <span
      role="status"
      aria-label={label ?? status}
      className={clsx('ui-status-dot', `status-${status}`, className)}
    />
  );
}
