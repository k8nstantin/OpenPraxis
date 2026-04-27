import { Toaster as SonnerToaster, toast as sonnerToast } from 'sonner';

// Single Toaster mount point. AppShell renders this once; everything
// else calls `toast.error(msg)` / `toast.success(msg)` from anywhere.
export function Toaster() {
  return (
    <SonnerToaster
      theme="dark"
      position="top-right"
      richColors
      closeButton
      toastOptions={{
        className: 'ui-toast',
      }}
    />
  );
}

export const toast = sonnerToast;
