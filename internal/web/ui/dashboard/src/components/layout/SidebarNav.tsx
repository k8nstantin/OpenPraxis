import { NavLink } from 'react-router-dom';
import clsx from 'clsx';
import { useUiStore } from '../../lib/stores/ui';
import type { TabRoute } from '../../routes';

export interface SidebarNavProps {
  tabs: TabRoute[];
}

export function SidebarNav({ tabs }: SidebarNavProps) {
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  return (
    <nav
      className={clsx('app-sidebar', collapsed && 'is-collapsed')}
      aria-label="Primary navigation"
    >
      <ul className="app-sidebar__list">
        {tabs.map((t) => (
          <li key={t.path}>
            <NavLink
              to={t.path}
              className={({ isActive }) =>
                clsx('app-sidebar__link', isActive && 'is-active')
              }
              end={t.path === '/'}
            >
              {t.icon && <span aria-hidden className="app-sidebar__icon">{t.icon}</span>}
              <span className="app-sidebar__label">{t.label}</span>
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
