import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

export interface BreadcrumbItem {
  label: ReactNode;
  to?: string;
}

export interface BreadcrumbProps {
  items: BreadcrumbItem[];
}

export function Breadcrumb({ items }: BreadcrumbProps) {
  return (
    <nav className="app-breadcrumb" aria-label="Breadcrumb">
      <ol>
        {items.map((item, idx) => {
          const isLast = idx === items.length - 1;
          return (
            <li key={idx} aria-current={isLast ? 'page' : undefined}>
              {item.to && !isLast ? <Link to={item.to}>{item.label}</Link> : <span>{item.label}</span>}
              {!isLast && <span className="app-breadcrumb__sep" aria-hidden="true">/</span>}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
