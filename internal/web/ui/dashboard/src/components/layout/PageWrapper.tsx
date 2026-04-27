import type { ReactNode } from 'react';
import clsx from 'clsx';
import { Breadcrumb, type Crumb } from './Breadcrumb';

export interface PageWrapperProps {
  title?: ReactNode;
  toolbar?: ReactNode;
  breadcrumb?: Crumb[];
  className?: string;
  children: ReactNode;
}

export function PageWrapper({ title, toolbar, breadcrumb, className, children }: PageWrapperProps) {
  return (
    <section className={clsx('app-page', className)}>
      {breadcrumb && <Breadcrumb items={breadcrumb} />}
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
