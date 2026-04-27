import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import clsx from 'clsx';
import type { ButtonSize, ButtonVariant } from './Button';

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Required: every icon-only button needs an accessible name. */
  'aria-label': string;
  icon: ReactNode;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { variant = 'ghost', size = 'md', icon, className, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={clsx('ui-icon-btn', `ui-btn--${variant}`, `ui-btn--${size}`, className)}
      {...rest}
    >
      {icon}
    </button>
  );
});
