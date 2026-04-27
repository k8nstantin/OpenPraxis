import { useMemo } from 'react';
import { Badge } from '@/components/ui/Badge';
import { useAllManifests, useUpdateManifestProject } from '@/lib/queries/manifests';
import { PickerDialog, type PickerDialogItem } from './PickerDialog';

export interface LinkManifestPickerProps {
  productId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LinkManifestPicker({ productId, open, onOpenChange }: LinkManifestPickerProps) {
  const all = useAllManifests();
  const link = useUpdateManifestProject();

  const items: PickerDialogItem[] = useMemo(() => {
    const list = (all.data ?? []).filter((m) => m.project_id !== productId);
    return list.map((m) => ({
      id: m.id,
      search: `${m.marker} ${m.title} ${m.status}`,
      render: (
        <span className="picker-row">
          <span className="marker">{m.marker}</span>
          <Badge tone={m.status === 'open' ? 'success' : m.status === 'closed' ? 'neutral' : 'info'}>{m.status}</Badge>
          <span className="picker-row__title">{m.title}</span>
        </span>
      ),
    }));
  }, [all.data, productId]);

  return (
    <PickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Link manifest to product"
      description="Click a manifest to link it to this product."
      busy={link.isPending}
      items={items}
      emptyMessage={all.isLoading ? 'Loading…' : 'No manifests available to link.'}
      onPick={(manifestId) =>
        link.mutate(
          { manifestId, productId },
          {
            onSuccess: () => onOpenChange(false),
          },
        )
      }
    />
  );
}
