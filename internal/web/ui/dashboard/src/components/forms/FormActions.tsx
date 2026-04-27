import type { ReactNode } from 'react';

export interface FormActionsProps {
  children: ReactNode;
  align?: 'start' | 'end';
}

export function FormActions({ children, align = 'end' }: FormActionsProps) {
  return <div className={`form-actions form-actions--${align}`}>{children}</div>;
}
