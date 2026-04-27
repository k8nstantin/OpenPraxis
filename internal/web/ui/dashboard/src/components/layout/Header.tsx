import { Link } from 'react-router-dom';
import { IconButton } from '@/components/ui/IconButton';
import { useUiStore } from '@/lib/stores/ui';

export function Header() {
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  return (
    <header className="app-header" role="banner">
      <IconButton
        icon={collapsed ? '☰' : '×'}
        label={collapsed ? 'Expand navigation' : 'Collapse navigation'}
        onClick={toggleSidebar}
        size="md"
      />
      <Link to="/" className="app-header__brand">
        <strong>OpenPraxis</strong>
        <span className="app-header__brand-tag">Dashboard v2</span>
      </Link>
      <div className="app-header__spacer" />
      <a className="app-header__legacy" href="/">
        Legacy UI
      </a>
    </header>
  );
}
