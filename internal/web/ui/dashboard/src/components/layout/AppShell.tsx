import type { ReactNode } from 'react';
import { Header } from './Header';
import { SidebarNav } from './SidebarNav';
import { useUiStore } from '@/lib/stores/ui';
import clsx from 'clsx';

export interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  return (
    <div className={clsx('app-shell', collapsed && 'is-collapsed')}>
      <Header />
      <SidebarNav />
      <main className="app-shell__main" id="main">
        {children}
      </main>
    </div>
  );
}
