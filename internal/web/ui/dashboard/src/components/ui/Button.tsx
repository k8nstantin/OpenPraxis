import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import clsx from 'clsx';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  leftIcon?: ReactNode;
  rightIcon?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading, disabled, leftIcon, rightIcon, className, children, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={clsx('ui-btn', `ui-btn--${variant}`, `ui-btn--${size}`, loading && 'is-loading', className)}
      {...rest}
    >
      {leftIcon && <span className="ui-btn__icon ui-btn__icon--left">{leftIcon}</span>}
      <span className="ui-btn__label">{children}</span>
      {rightIcon && <span className="ui-btn__icon ui-btn__icon--right">{rightIcon}</span>}
    </button>
  );
});
