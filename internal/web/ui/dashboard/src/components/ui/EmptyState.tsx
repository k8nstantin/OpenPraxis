import type { ReactNode } from 'react';
import clsx from 'clsx';

export interface EmptyStateProps {
  title?: ReactNode;
  message: ReactNode;
  action?: ReactNode;
  tone?: 'neutral' | 'error';
  className?: string;
}

export function EmptyState({ title, message, action, tone = 'neutral', className }: EmptyStateProps) {
  return (
    <div className={clsx('ui-empty', `ui-empty--${tone}`, className)} role={tone === 'error' ? 'alert' : 'status'}>
      {title && <div className="ui-empty__title">{title}</div>}
      <div className="ui-empty__message">{message}</div>
      {action && <div className="ui-empty__action">{action}</div>}
    </div>
  );
}
