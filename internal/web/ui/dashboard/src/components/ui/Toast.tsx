// Thin wrapper around `sonner` so the rest of the app can `import { toast,
// Toaster } from '@/components/ui/Toast'` without binding to the underlying
// library at every callsite. Swap the import here if we ever change toast
// implementations.
import { Toaster as SonnerToaster, toast as sonnerToast } from 'sonner';

export const toast = sonnerToast;

export function Toaster() {
  return <SonnerToaster theme="dark" position="bottom-right" richColors closeButton />;
}
