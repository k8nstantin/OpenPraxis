import { Link } from 'react-router-dom';
import { useUiStore } from '../../lib/stores/ui';
import { IconButton } from '../ui/IconButton';

export function Header() {
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);
  return (
    <header className="app-header" role="banner">
      <IconButton
        aria-label="Toggle sidebar"
        icon={<span aria-hidden>☰</span>}
        onClick={toggleSidebar}
        className="app-header__menu"
      />
      <Link to="/" className="app-header__brand" aria-label="OpenPraxis dashboard home">
        <span className="app-header__brand-mark">◈</span>
        <span className="app-header__brand-name">OpenPraxis</span>
      </Link>
      <div className="app-header__spacer" />
      <a href="/" className="app-header__legacy-link" title="Switch to legacy UI">
        Legacy UI
      </a>
    </header>
  );
}
