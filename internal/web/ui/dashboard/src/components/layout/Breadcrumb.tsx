import { Link } from 'react-router-dom';
import type { ReactNode } from 'react';

export interface Crumb {
  label: ReactNode;
  to?: string;
}

export interface BreadcrumbProps {
  items: Crumb[];
}

export function Breadcrumb({ items }: BreadcrumbProps) {
  if (!items.length) return null;
  return (
    <nav aria-label="Breadcrumb" className="app-breadcrumb">
      <ol className="app-breadcrumb__list">
        {items.map((c, i) => {
          const isLast = i === items.length - 1;
          return (
            <li key={i} className="app-breadcrumb__item">
              {c.to && !isLast ? <Link to={c.to}>{c.label}</Link> : <span aria-current={isLast ? 'page' : undefined}>{c.label}</span>}
              {!isLast && <span aria-hidden className="app-breadcrumb__sep">›</span>}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
