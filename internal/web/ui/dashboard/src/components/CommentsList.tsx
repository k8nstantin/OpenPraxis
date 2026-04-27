import clsx from 'clsx';
import { EmptyState } from './ui/EmptyState';
import { MarkdownRenderer } from './MarkdownRenderer';

export interface Comment {
  id: string;
  author: string;
  body: string;
  body_html?: string;
  created_at: string;
}

export interface CommentsListProps {
  comments: Comment[] | undefined;
  loading?: boolean;
  className?: string;
}

export function CommentsList({ comments, loading, className }: CommentsListProps) {
  if (loading) return <EmptyState tone="loading" title="Loading comments…" />;
  if (!comments || comments.length === 0) {
    return <EmptyState description="No comments yet." />;
  }
  return (
    <ul className={clsx('comments-list', className)} aria-label="Comments">
      {comments.map((c) => (
        <li key={c.id} className="comments-list__item">
          <header className="comments-list__meta">
            <span className="comments-list__author">{c.author}</span>
            <time className="comments-list__time" dateTime={c.created_at}>
              {c.created_at}
            </time>
          </header>
          <MarkdownRenderer source={c.body_html ?? c.body} className="comments-list__body" />
        </li>
      ))}
    </ul>
  );
}
