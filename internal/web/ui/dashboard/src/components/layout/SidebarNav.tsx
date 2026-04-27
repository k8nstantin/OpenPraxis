import { NavLink } from 'react-router-dom';
import clsx from 'clsx';
import { routeRegistry } from '@/routes';
import { useUiStore } from '@/lib/stores/ui';

export function SidebarNav() {
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  return (
    <nav
      className={clsx('app-sidebar', collapsed && 'is-collapsed')}
      aria-label="Primary"
    >
      <ul className="app-sidebar__list">
        {routeRegistry
          .filter((r) => r.nav)
          .map((r) => (
            <li key={r.path}>
              <NavLink
                to={r.path}
                end={r.path === '/'}
                className={({ isActive }) => clsx('app-sidebar__link', isActive && 'is-active')}
              >
                <span className="app-sidebar__icon" aria-hidden="true">{r.icon ?? '•'}</span>
                <span className="app-sidebar__label">{r.label}</span>
              </NavLink>
            </li>
          ))}
      </ul>
    </nav>
  );
}
