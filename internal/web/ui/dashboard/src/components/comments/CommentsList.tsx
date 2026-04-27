// Read-only comments list. Editing/soft-delete UIs land in tab-specific
// manifests (out of scope for M1 / Foundation per the manifest spec).
import { MarkdownRenderer } from '@/components/desc/MarkdownRenderer';
import { EmptyState } from '@/components/ui/EmptyState';
import type { Comment } from '@/lib/types';

export interface CommentsListProps {
  comments: Comment[];
  loading?: boolean;
}

export function CommentsList({ comments, loading }: CommentsListProps) {
  if (loading) return <EmptyState message="Loading comments…" />;
  if (!comments.length) return <EmptyState message="No comments yet." />;
  return (
    <ul className="comments-list" aria-label="Comments">
      {comments.map((c) => (
        <li key={c.id} className="comments-list__item">
          <header className="comments-list__head">
            <span className="comments-list__author">{c.author ?? 'unknown'}</span>
            {c.created_at && (
              <time dateTime={c.created_at} className="comments-list__time">
                {new Date(c.created_at).toLocaleString()}
              </time>
            )}
          </header>
          <MarkdownRenderer source={c.body ?? ''} html={c.body_html} className="comments-list__body" />
        </li>
      ))}
    </ul>
  );
}
