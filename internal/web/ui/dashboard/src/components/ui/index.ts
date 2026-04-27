// Barrel for the design-system primitives. Tab manifests should import
// from `@/components/ui` and never reach into individual files — that
// keeps the component library refactorable in place.
export { Button, type ButtonProps, type ButtonVariant, type ButtonSize } from './Button';
export { IconButton, type IconButtonProps } from './IconButton';
export { Badge, type BadgeProps, type BadgeTone } from './Badge';
export { StatusDot, type StatusDotProps, type StatusKind } from './StatusDot';
export { EmptyState, type EmptyStateProps } from './EmptyState';
export { ErrorBoundary } from './ErrorBoundary';
export { Dialog, type DialogProps } from './Dialog';
export { Tooltip, TooltipProvider, type TooltipProps } from './Tooltip';
export { Popover, type PopoverProps } from './Popover';
export { Toaster, toast } from './Toast';
