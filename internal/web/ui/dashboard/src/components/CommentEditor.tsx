import { useState, type FormEvent } from 'react';
import { Button } from './ui/Button';
import { FormField } from './forms/FormField';
import { FormActions } from './forms/FormActions';

export interface CommentEditorProps {
  onSubmit: (body: string) => void | Promise<void>;
  placeholder?: string;
  submitLabel?: string;
  disabled?: boolean;
}

export function CommentEditor({
  onSubmit,
  placeholder = 'Add a comment…',
  submitLabel = 'Comment',
  disabled,
}: CommentEditorProps) {
  const [body, setBody] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    setSubmitting(true);
    try {
      await onSubmit(body);
      setBody('');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="comment-editor" onSubmit={handleSubmit}>
      <FormField
        as="textarea"
        label="New comment"
        rows={3}
        value={body}
        placeholder={placeholder}
        onChange={(e) => setBody((e.target as HTMLTextAreaElement).value)}
        disabled={disabled || submitting}
      />
      <FormActions>
        <Button type="submit" variant="primary" loading={submitting} disabled={disabled || !body.trim()}>
          {submitLabel}
        </Button>
      </FormActions>
    </form>
  );
}
