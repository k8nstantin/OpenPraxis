import { useId, type ReactElement, cloneElement } from 'react';
import { Tooltip as RTooltip } from 'react-tooltip';

export interface TooltipProps {
  content: string;
  children: ReactElement;
  placement?: 'top' | 'bottom' | 'left' | 'right';
}

// Wraps a single trigger element with an accessible tooltip via
// react-tooltip. The trigger MUST be focusable (button, link, etc.) for
// keyboard users to read the tip — we don't add tabIndex automatically.
export function Tooltip({ content, children, placement = 'top' }: TooltipProps) {
  const id = useId();
  const trigger = cloneElement(children, {
    'data-tooltip-id': id,
    'data-tooltip-content': content,
  });
  return (
    <>
      {trigger}
      <RTooltip id={id} place={placement} className="ui-tooltip" />
    </>
  );
}
