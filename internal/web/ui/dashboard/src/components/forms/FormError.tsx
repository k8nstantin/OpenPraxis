import type { ReactNode } from 'react';

export interface FormErrorProps {
  id?: string;
  children: ReactNode;
}

export function FormError({ id, children }: FormErrorProps) {
  return (
    <p id={id} role="alert" className="form-error">
      {children}
    </p>
  );
}
