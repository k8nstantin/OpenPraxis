// Minimal comment editor. Markdown toolbar / preview tab is intentionally
// deferred to a tab-specific manifest — this component only owns the
// textarea, character counter, and submit/cancel buttons.
import { useState } from 'react';
import { Button } from '@/components/ui/Button';

export interface CommentEditorProps {
  initialValue?: string;
  placeholder?: string;
  submitLabel?: string;
  onSubmit: (body: string) => void | Promise<void>;
  onCancel?: () => void;
  busy?: boolean;
  maxLength?: number;
}

export function CommentEditor({
  initialValue = '',
  placeholder = 'Write a comment…',
  submitLabel = 'Post comment',
  onSubmit,
  onCancel,
  busy,
  maxLength = 4000,
}: CommentEditorProps) {
  const [value, setValue] = useState(initialValue);
  const trimmed = value.trim();
  const tooLong = value.length > maxLength;

  return (
    <form
      className="comment-editor"
      onSubmit={(e) => {
        e.preventDefault();
        if (!trimmed || tooLong || busy) return;
        void onSubmit(trimmed);
      }}
    >
      <textarea
        className="comment-editor__textarea"
        placeholder={placeholder}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        rows={4}
        disabled={busy}
        aria-invalid={tooLong || undefined}
      />
      <div className="comment-editor__footer">
        <span className={'comment-editor__count' + (tooLong ? ' is-over' : '')}>
          {value.length} / {maxLength}
        </span>
        <div className="comment-editor__actions">
          {onCancel && (
            <Button type="button" variant="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
          )}
          <Button type="submit" variant="primary" disabled={!trimmed || tooLong} loading={busy}>
            {submitLabel}
          </Button>
        </div>
      </div>
    </form>
  );
}
