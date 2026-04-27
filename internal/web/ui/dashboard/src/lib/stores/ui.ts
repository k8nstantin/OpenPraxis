import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type DescMode = 'markup' | 'rendered';
export type Theme = 'dark' | 'light';

interface UiState {
  sidebarCollapsed: boolean;
  descMode: DescMode;
  theme: Theme;
  toggleSidebar: () => void;
  setSidebarCollapsed: (v: boolean) => void;
  setDescMode: (m: DescMode) => void;
  setTheme: (t: Theme) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      descMode: 'rendered',
      theme: 'dark',
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarCollapsed: (v) => set({ sidebarCollapsed: v }),
      setDescMode: (m) => set({ descMode: m }),
      setTheme: (t) => set({ theme: t }),
    }),
    {
      name: 'openpraxis.ui',
      version: 1,
    },
  ),
);
