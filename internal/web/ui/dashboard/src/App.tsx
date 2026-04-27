import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppShell } from './components/layout/AppShell';
import { TAB_ROUTES } from './routes';

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
      <BrowserRouter basename="/dashboard">
        <AppShell>
          <Routes>
            {TAB_ROUTES.flatMap((tab) => {
              const Comp = tab.component;
              const paths = [tab.path, ...(tab.subPaths ?? [])];
              return paths.map((p) => (
                <Route key={p} path={p} element={<Comp />} />
              ));
            })}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
