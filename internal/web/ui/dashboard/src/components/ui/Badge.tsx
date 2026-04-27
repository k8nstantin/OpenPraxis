import type { HTMLAttributes, ReactNode } from 'react';
import clsx from 'clsx';

export type BadgeTone = 'neutral' | 'info' | 'success' | 'warn' | 'danger' | 'tag';

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
  children: ReactNode;
}

export function Badge({ tone = 'neutral', className, children, ...rest }: BadgeProps) {
  return (
    <span className={clsx('ui-badge', `ui-badge--${tone}`, className)} {...rest}>
      {children}
    </span>
  );
}
