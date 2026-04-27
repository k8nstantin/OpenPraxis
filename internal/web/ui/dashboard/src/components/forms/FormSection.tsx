import type { ReactNode } from 'react';

export interface FormSectionProps {
  title?: ReactNode;
  description?: ReactNode;
  children: ReactNode;
}

export function FormSection({ title, description, children }: FormSectionProps) {
  return (
    <fieldset className="ui-form-section">
      {title && <legend className="ui-form-section__title">{title}</legend>}
      {description && <div className="ui-form-section__description">{description}</div>}
      <div className="ui-form-section__body">{children}</div>
    </fieldset>
  );
}
