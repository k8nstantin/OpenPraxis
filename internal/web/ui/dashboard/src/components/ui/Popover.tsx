import * as RP from '@radix-ui/react-popover';
import clsx from 'clsx';
import type { ReactNode } from 'react';

export interface PopoverProps {
  trigger: ReactNode;
  children: ReactNode;
  align?: 'start' | 'center' | 'end';
  side?: 'top' | 'bottom' | 'left' | 'right';
  className?: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function Popover({ trigger, children, align = 'start', side = 'bottom', className, open, onOpenChange }: PopoverProps) {
  return (
    <RP.Root open={open} onOpenChange={onOpenChange}>
      <RP.Trigger asChild>{trigger}</RP.Trigger>
      <RP.Portal>
        <RP.Content
          align={align}
          side={side}
          sideOffset={6}
          className={clsx('ui-popover', className)}
        >
          {children}
          <RP.Arrow className="ui-popover__arrow" />
        </RP.Content>
      </RP.Portal>
    </RP.Root>
  );
}

export const PopoverClose = RP.Close;
