import type { ReactNode } from 'react';
import clsx from 'clsx';
import { Breadcrumb, type BreadcrumbItem } from './Breadcrumb';

export interface PageWrapperProps {
  title?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  toolbar?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function PageWrapper({ title, breadcrumbs, toolbar, children, className }: PageWrapperProps) {
  return (
    <section className={clsx('app-page', className)}>
      {breadcrumbs && breadcrumbs.length > 0 && <Breadcrumb items={breadcrumbs} />}
      {(title || toolbar) && (
        <header className="app-page__header">
          {title && <h1 className="app-page__title">{title}</h1>}
          {toolbar && <div className="app-page__toolbar">{toolbar}</div>}
        </header>
      )}
      <div className="app-page__body">{children}</div>
    </section>
  );
}
