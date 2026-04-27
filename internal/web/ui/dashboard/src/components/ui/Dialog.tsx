import * as RD from '@radix-ui/react-dialog';
import clsx from 'clsx';
import type { ReactNode } from 'react';

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function Dialog({ open, onOpenChange, title, description, children, footer, size = 'md', className }: DialogProps) {
  return (
    <RD.Root open={open} onOpenChange={onOpenChange}>
      <RD.Portal>
        <RD.Overlay className="ui-dialog__overlay" />
        <RD.Content className={clsx('ui-dialog', `ui-dialog--${size}`, className)}>
          <header className="ui-dialog__header">
            <RD.Title className="ui-dialog__title">{title}</RD.Title>
            {description && (
              <RD.Description className="ui-dialog__description">{description}</RD.Description>
            )}
          </header>
          <div className="ui-dialog__body">{children}</div>
          {footer && <footer className="ui-dialog__footer">{footer}</footer>}
          <RD.Close asChild>
            <button className="ui-dialog__close" type="button" aria-label="Close dialog">×</button>
          </RD.Close>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  );
}

export const DialogClose = RD.Close;
