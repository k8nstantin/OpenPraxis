import clsx from 'clsx';
import type { ReactNode } from 'react';

export interface EmptyStateProps {
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  tone?: 'neutral' | 'error' | 'loading';
  icon?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, action, tone = 'neutral', icon, className }: EmptyStateProps) {
  return (
    <div className={clsx('ui-empty-state', `ui-empty-state--${tone}`, className)} role={tone === 'error' ? 'alert' : undefined}>
      {icon && <div className="ui-empty-state__icon">{icon}</div>}
      {title && <div className="ui-empty-state__title">{title}</div>}
      {description && <div className="ui-empty-state__description">{description}</div>}
      {action && <div className="ui-empty-state__action">{action}</div>}
    </div>
  );
}
