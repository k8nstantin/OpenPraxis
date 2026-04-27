import { useMemo } from 'react';
import { Badge } from '@/components/ui/Badge';
import {
  useAddProductDependency,
  useAllProductsFlat,
} from '@/lib/queries/products';
import type { Product, ProductDepEntry } from '@/lib/types';
import { PickerDialog, type PickerDialogItem } from './PickerDialog';

export interface AddDepPickerProps {
  product: Product;
  existingDeps: ProductDepEntry[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AddDepPicker({ product, existingDeps, open, onOpenChange }: AddDepPickerProps) {
  const all = useAllProductsFlat();
  const add = useAddProductDependency(product.id);

  const items: PickerDialogItem[] = useMemo(() => {
    const depIds = new Set(existingDeps.map((d) => d.id));
    const list = (all.data ?? []).filter(
      (p) => p.id !== product.id && !depIds.has(p.id) && p.status !== 'archive',
    );
    return list.map((p) => ({
      id: p.id,
      search: `${p.marker} ${p.title} ${p.status}`,
      render: (
        <span className="picker-row">
          <span className="marker">{p.marker}</span>
          <Badge tone={p.status === 'open' ? 'success' : p.status === 'closed' ? 'neutral' : 'info'}>
            {p.status}
          </Badge>
          <span className="picker-row__title">{p.title}</span>
        </span>
      ),
    }));
  }, [all.data, existingDeps, product.id]);

  return (
    <PickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Add dependency for ${product.title}`}
      description="Select a product this product depends on."
      busy={add.isPending}
      items={items}
      emptyMessage={all.isLoading ? 'Loading…' : 'No eligible products to depend on.'}
      onPick={(depId) => add.mutate(depId, { onSuccess: () => onOpenChange(false) })}
    />
  );
}
