import type { ReactNode } from 'react';

export interface FormErrorProps {
  children: ReactNode;
}

export function FormError({ children }: FormErrorProps) {
  if (!children) return null;
  return <div role="alert" className="ui-form-error">{children}</div>;
}
