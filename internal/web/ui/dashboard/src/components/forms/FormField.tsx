import { useId, type ReactNode, type InputHTMLAttributes, type TextareaHTMLAttributes, forwardRef } from 'react';
import clsx from 'clsx';

interface BaseProps {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  required?: boolean;
  className?: string;
}

export type FormFieldProps =
  | (BaseProps & { as?: 'input' } & InputHTMLAttributes<HTMLInputElement>)
  | (BaseProps & { as: 'textarea' } & TextareaHTMLAttributes<HTMLTextAreaElement>);

// Forwarded ref handles both <input> and <textarea>. Spread
// register('field') from react-hook-form straight onto FormField.
export const FormField = forwardRef<HTMLInputElement | HTMLTextAreaElement, FormFieldProps>(
  function FormField(props, ref) {
    const id = useId();
    const { label, hint, error, required, className, as = 'input', ...rest } = props as BaseProps & {
      as?: 'input' | 'textarea';
    } & Record<string, unknown>;
    const describedBy = [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ') || undefined;
    const isTextarea = as === 'textarea';
    return (
      <div className={clsx('ui-form-field', error && 'is-invalid', className)}>
        <label htmlFor={id} className="ui-form-field__label">
          {label}
          {required && <span aria-hidden className="ui-form-field__required">*</span>}
        </label>
        {isTextarea ? (
          <textarea
            id={id}
            ref={ref as React.Ref<HTMLTextAreaElement>}
            aria-invalid={Boolean(error) || undefined}
            aria-describedby={describedBy}
            aria-required={required || undefined}
            className="ui-form-field__control"
            {...(rest as TextareaHTMLAttributes<HTMLTextAreaElement>)}
          />
        ) : (
          <input
            id={id}
            ref={ref as React.Ref<HTMLInputElement>}
            aria-invalid={Boolean(error) || undefined}
            aria-describedby={describedBy}
            aria-required={required || undefined}
            className="ui-form-field__control"
            {...(rest as InputHTMLAttributes<HTMLInputElement>)}
          />
        )}
        {hint && !error && <div id={`${id}-hint`} className="ui-form-field__hint">{hint}</div>}
        {error && <div id={`${id}-error`} role="alert" className="ui-form-field__error">{error}</div>}
      </div>
    );
  },
);
