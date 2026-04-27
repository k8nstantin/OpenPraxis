import * as RadixPopover from '@radix-ui/react-popover';
import type { ReactNode } from 'react';

export interface PopoverProps {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  align?: 'start' | 'center' | 'end';
  side?: 'top' | 'right' | 'bottom' | 'left';
  /**
   * Accessible name for the popover content. Radix sets `role="dialog"` on
   * the rendered content; without an aria-label or labelled element axe
   * flags `aria-dialog-name`. Required for non-trivial popovers; optional
   * if the popover content already contains a heading or labelled control.
   */
  ariaLabel?: string;
}

export function Popover({ trigger, children, open, onOpenChange, align = 'start', side = 'bottom', ariaLabel }: PopoverProps) {
  return (
    <RadixPopover.Root open={open} onOpenChange={onOpenChange}>
      <RadixPopover.Trigger asChild>{trigger}</RadixPopover.Trigger>
      <RadixPopover.Portal>
        <RadixPopover.Content
          side={side}
          align={align}
          sideOffset={6}
          className="ui-popover"
          aria-label={ariaLabel}
        >
          {children}
          <RadixPopover.Arrow className="ui-popover__arrow" />
        </RadixPopover.Content>
      </RadixPopover.Portal>
    </RadixPopover.Root>
  );
}
