import { Suspense, useEffect, type ReactNode } from 'react';
import { Header } from './Header';
import { SidebarNav } from './SidebarNav';
import { ErrorBoundary } from '../ui/ErrorBoundary';
import { Toaster, toast } from '../ui/Toast';
import { onApiError } from '../../lib/api';
import { TAB_ROUTES } from '../../routes';

export interface AppShellProps {
  children: ReactNode;
}

// Top-level frame for every page. Owns the header, primary nav, the
// global error toast bridge (every ApiError funnels through here), and
// a single ErrorBoundary so a render crash in one tab doesn't blank
// the whole dashboard.
export function AppShell({ children }: AppShellProps) {
  useEffect(() => {
    return onApiError((err) => {
      toast.error(err.message, {
        description: err.requestId ? `request ${err.requestId}` : undefined,
      });
    });
  }, []);

  return (
    <div className="app-shell">
      <Header />
      <div className="app-shell__body">
        <SidebarNav tabs={TAB_ROUTES} />
        <main className="app-shell__main" id="main">
          <ErrorBoundary>
            <Suspense fallback={<div className="page-loading">Loading…</div>}>
              {children}
            </Suspense>
          </ErrorBoundary>
        </main>
      </div>
      <Toaster />
    </div>
  );
}
