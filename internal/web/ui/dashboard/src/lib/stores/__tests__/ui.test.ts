import { beforeEach, describe, expect, it } from 'vitest';
import { useUiStore } from '../ui';

describe('useUiStore', () => {
  beforeEach(() => {
    // Reset between tests — Zustand stores survive module imports.
    useUiStore.setState({ sidebarCollapsed: false, descMode: 'rendered', theme: 'dark' });
  });

  it('toggleSidebar flips the boolean', () => {
    expect(useUiStore.getState().sidebarCollapsed).toBe(false);
    useUiStore.getState().toggleSidebar();
    expect(useUiStore.getState().sidebarCollapsed).toBe(true);
    useUiStore.getState().toggleSidebar();
    expect(useUiStore.getState().sidebarCollapsed).toBe(false);
  });

  it('setDescMode updates descMode', () => {
    useUiStore.getState().setDescMode('markup');
    expect(useUiStore.getState().descMode).toBe('markup');
  });

  it('persists to localStorage under openpraxis.ui', () => {
    useUiStore.getState().setDescMode('markup');
    const raw = localStorage.getItem('openpraxis.ui');
    expect(raw).toBeTruthy();
    const parsed = JSON.parse(raw as string);
    expect(parsed.state.descMode).toBe('markup');
  });
});
