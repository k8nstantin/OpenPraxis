import { afterEach, describe, expect, it } from 'vitest';
import { useUiStore } from '../ui';

const initial = useUiStore.getState();

describe('useUiStore', () => {
  afterEach(() => {
    useUiStore.setState(initial, true);
    localStorage.clear();
  });

  it('toggleSidebar flips the collapsed flag', () => {
    expect(useUiStore.getState().sidebarCollapsed).toBe(false);
    useUiStore.getState().toggleSidebar();
    expect(useUiStore.getState().sidebarCollapsed).toBe(true);
    useUiStore.getState().toggleSidebar();
    expect(useUiStore.getState().sidebarCollapsed).toBe(false);
  });

  it('setDescMode persists to localStorage', () => {
    useUiStore.getState().setDescMode('rendered');
    const stored = localStorage.getItem('openpraxis.ui');
    expect(stored).toBeTruthy();
    const parsed = JSON.parse(stored!);
    expect(parsed.state.descMode).toBe('rendered');
  });
});
