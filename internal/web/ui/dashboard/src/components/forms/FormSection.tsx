import type { ReactNode } from 'react';

export interface FormSectionProps {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
}

export function FormSection({ title, description, children }: FormSectionProps) {
  return (
    <section className="form-section">
      <header className="form-section__header">
        <h3 className="form-section__title">{title}</h3>
        {description && <p className="form-section__description">{description}</p>}
      </header>
      <div className="form-section__body">{children}</div>
    </section>
  );
}
