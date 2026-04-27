import * as RadixDialog from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import clsx from 'clsx';

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg';
}

export function Dialog({ open, onOpenChange, title, description, children, footer, size = 'md' }: DialogProps) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className="ui-dialog__overlay" />
        <RadixDialog.Content className={clsx('ui-dialog', `ui-dialog--${size}`)}>
          <RadixDialog.Title className="ui-dialog__title">{title}</RadixDialog.Title>
          {description && <RadixDialog.Description className="ui-dialog__description">{description}</RadixDialog.Description>}
          <div className="ui-dialog__body">{children}</div>
          {footer && <div className="ui-dialog__footer">{footer}</div>}
          <RadixDialog.Close aria-label="Close" className="ui-dialog__close">×</RadixDialog.Close>
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
