import { useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { useProductIdeas, useUpdateIdeaProject } from '@/lib/queries/ideas';
import { LinkIdeaPicker } from './LinkIdeaPicker';

export interface LinkedIdeasProps {
  productId: string;
}

export function LinkedIdeas({ productId }: LinkedIdeasProps) {
  const q = useProductIdeas(productId);
  const unlink = useUpdateIdeaProject();
  const [pickerOpen, setPickerOpen] = useState(false);

  return (
    <section className="linked-ideas" aria-labelledby="linked-ideas-title">
      <header className="linked-manifests__header">
        <h3 id="linked-ideas-title">
          Linked Ideas {q.data ? <span className="sub-count">({q.data.length})</span> : null}
        </h3>
        <Button size="sm" variant="ghost" onClick={() => setPickerOpen(true)}>
          + Link Idea
        </Button>
      </header>
      {q.isLoading && <EmptyState message="Loading…" />}
      {q.data && q.data.length === 0 && <EmptyState message="No ideas linked yet." />}
      {q.data && q.data.length > 0 && (
        <ul className="linked-ideas__list" role="list">
          {q.data.map((idea) => (
            <li key={idea.id} className="linked-ideas__row">
              <span className="marker">{idea.marker}</span>
              <Badge>{idea.status}</Badge>
              <Badge tone={idea.priority === 'critical' ? 'danger' : idea.priority === 'high' ? 'warn' : 'neutral'}>
                {idea.priority}
              </Badge>
              <span className="linked-ideas__title">{idea.title}</span>
              <button
                type="button"
                className="linked-ideas__rm"
                disabled={unlink.isPending}
                aria-label={`Unlink idea ${idea.title}`}
                onClick={() => {
                  if (window.confirm(`Unlink idea "${idea.title}" from product?`)) {
                    unlink.mutate({ idea, productId: '' });
                  }
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <LinkIdeaPicker productId={productId} open={pickerOpen} onOpenChange={setPickerOpen} />
    </section>
  );
}
