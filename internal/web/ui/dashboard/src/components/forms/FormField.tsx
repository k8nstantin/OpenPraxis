import type { ReactNode } from 'react';
import clsx from 'clsx';
import { FormError } from './FormError';

export interface FormFieldProps {
  label: ReactNode;
  htmlFor: string;
  hint?: ReactNode;
  error?: string;
  required?: boolean;
  className?: string;
  children: ReactNode;
}

export function FormField({ label, htmlFor, hint, error, required, className, children }: FormFieldProps) {
  return (
    <div className={clsx('form-field', error && 'is-invalid', className)}>
      <label htmlFor={htmlFor} className="form-field__label">
        {label}
        {required && <span aria-hidden="true" className="form-field__req">*</span>}
      </label>
      {children}
      {hint && !error && <p className="form-field__hint">{hint}</p>}
      {error && <FormError id={`${htmlFor}-error`}>{error}</FormError>}
    </div>
  );
}
