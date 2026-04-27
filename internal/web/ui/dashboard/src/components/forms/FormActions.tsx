import type { ReactNode } from 'react';

export interface FormActionsProps {
  align?: 'left' | 'right' | 'between';
  children: ReactNode;
}

export function FormActions({ align = 'right', children }: FormActionsProps) {
  return <div className={`ui-form-actions ui-form-actions--${align}`}>{children}</div>;
}
