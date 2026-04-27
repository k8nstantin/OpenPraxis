import { Suspense } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { routeRegistry } from './routes';
import { AppShell } from './components/layout';
import { ErrorBoundary, Toaster } from './components/ui';
import { TooltipProvider } from './components/ui/Tooltip';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <BrowserRouter basename="/dashboard">
          <ErrorBoundary>
            <AppShell>
              <Suspense fallback={<div className="page-loading">Loading…</div>}>
                <Routes>
                  {routeRegistry.map((r) => (
                    <Route key={r.path} path={r.path} element={<r.component />} />
                  ))}
                  {/* Detail variant for products — kept here rather than in
                      the registry because the registry powers nav-rendering
                      and a /products/:id link in the sidebar would be wrong. */}
                  <Route path="/products/:id" element={(() => {
                    const r = routeRegistry.find((x) => x.path === '/products');
                    if (!r) return null;
                    const C = r.component;
                    return <C />;
                  })()} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Routes>
              </Suspense>
            </AppShell>
          </ErrorBoundary>
        </BrowserRouter>
      </TooltipProvider>
      <Toaster />
    </QueryClientProvider>
  );
}
