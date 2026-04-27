import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type DescMode = 'markup' | 'rendered';
export type Theme = 'dark';

interface UiState {
  sidebarCollapsed: boolean;
  descMode: DescMode;
  theme: Theme;
  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setDescMode: (mode: DescMode) => void;
  setTheme: (theme: Theme) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      descMode: 'markup',
      theme: 'dark',
      setSidebarCollapsed: (v) => set({ sidebarCollapsed: v }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setDescMode: (mode) => set({ descMode: mode }),
      setTheme: (theme) => set({ theme }),
    }),
    {
      name: 'openpraxis.ui',
      // Bump when a breaking shape change lands.
      version: 1,
    },
  ),
);
