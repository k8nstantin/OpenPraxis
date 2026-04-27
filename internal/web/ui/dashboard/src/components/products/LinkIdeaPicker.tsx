import { useMemo } from 'react';
import { Badge } from '@/components/ui/Badge';
import { useAllIdeas, useUpdateIdeaProject } from '@/lib/queries/ideas';
import { PickerDialog, type PickerDialogItem } from './PickerDialog';

export interface LinkIdeaPickerProps {
  productId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LinkIdeaPicker({ productId, open, onOpenChange }: LinkIdeaPickerProps) {
  const all = useAllIdeas();
  const link = useUpdateIdeaProject();

  const items: PickerDialogItem[] = useMemo(() => {
    const list = (all.data ?? []).filter((i) => i.project_id !== productId);
    return list.map((idea) => ({
      id: idea.id,
      search: `${idea.marker} ${idea.title} ${idea.status} ${idea.priority}`,
      render: (
        <span className="picker-row">
          <span className="marker">{idea.marker}</span>
          <Badge>{idea.status}</Badge>
          <Badge tone={idea.priority === 'critical' ? 'danger' : idea.priority === 'high' ? 'warn' : 'neutral'}>
            {idea.priority}
          </Badge>
          <span className="picker-row__title">{idea.title}</span>
        </span>
      ),
    }));
  }, [all.data, productId]);

  return (
    <PickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Link idea to product"
      description="Click an idea to link it to this product."
      busy={link.isPending}
      items={items}
      emptyMessage={all.isLoading ? 'Loading…' : 'No ideas available to link.'}
      onPick={(ideaId) => {
        const idea = (all.data ?? []).find((i) => i.id === ideaId);
        if (!idea) return;
        link.mutate(
          { idea, productId },
          {
            onSuccess: () => onOpenChange(false),
          },
        );
      }}
    />
  );
}
