import { useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import {
  useProductDependencies,
  useRemoveProductDependency,
} from '@/lib/queries/products';
import type { Product, ProductDepEntry } from '@/lib/types';
import { AddDepPicker } from './AddDepPicker';

const TERMINAL_STATUSES = new Set(['closed', 'archive']);

export interface ProductDepsProps {
  product: Product;
  onNavigate: (productId: string) => void;
}

export function ProductDeps({ product, onNavigate }: ProductDepsProps) {
  const depsQ = useProductDependencies(product.id);
  const remove = useRemoveProductDependency(product.id);
  const [pickerOpen, setPickerOpen] = useState(false);

  const deps = depsQ.data?.deps ?? [];
  const dependents = depsQ.data?.dependents ?? [];
  const unsatisfied = deps.filter((d) => !TERMINAL_STATUSES.has(d.status));
  const satisfiedKind = deps.length === 0 ? 'none' : unsatisfied.length === 0 ? 'satisfied' : 'waiting';

  return (
    <section className="product-deps" aria-labelledby="product-deps-title">
      <header className="product-deps__header">
        <h3 id="product-deps-title" className="product-deps__title">Depends on</h3>
        {satisfiedKind === 'satisfied' && (
          <Badge tone="success" aria-label="all dependencies satisfied">
            ✓ SATISFIED
          </Badge>
        )}
        {satisfiedKind === 'waiting' && (
          <Badge
            tone="warn"
            title={`Waiting on: ${unsatisfied.map((u) => u.marker).join(', ')}`}
            aria-label={`waiting on ${unsatisfied.length} dependencies`}
          >
            ⏳ WAITING ON {unsatisfied.length}
          </Badge>
        )}
        <Button size="sm" variant="ghost" onClick={() => setPickerOpen(true)}>
          + Depends On
        </Button>
      </header>
      <div className="product-deps__pills" aria-label="Dependencies">
        {deps.length === 0 && (
          <span className="product-deps__empty">No dependencies</span>
        )}
        {deps.map((d) => (
          <DepPill
            key={d.id}
            dep={d}
            onNav={() => onNavigate(d.id)}
            onRemove={() => remove.mutate(d.id)}
            removing={remove.isPending}
          />
        ))}
      </div>
      {dependents.length > 0 && (
        <div className="product-deps__dependents">
          <h4 className="product-deps__sub">Depended on by</h4>
          <div className="product-deps__pills">
            {dependents.map((d) => (
              <DepPill key={d.id} dep={d} onNav={() => onNavigate(d.id)} />
            ))}
          </div>
        </div>
      )}
      {pickerOpen && (
        <AddDepPicker
          product={product}
          existingDeps={deps}
          open={pickerOpen}
          onOpenChange={setPickerOpen}
        />
      )}
    </section>
  );
}

function DepPill({
  dep,
  onNav,
  onRemove,
  removing,
}: {
  dep: ProductDepEntry;
  onNav: () => void;
  onRemove?: () => void;
  removing?: boolean;
}) {
  return (
    <span className={`product-dep-pill product-dep-pill--${dep.status}`}>
      <button type="button" className="product-dep-pill__nav" onClick={onNav}>
        {dep.marker}
      </button>
      <span className="product-dep-pill__title">{dep.title}</span>
      <span className="product-dep-pill__status">{dep.status}</span>
      {onRemove && (
        <button
          type="button"
          className="product-dep-pill__rm"
          aria-label={`Remove ${dep.marker} dependency`}
          disabled={removing}
          onClick={onRemove}
        >
          ×
        </button>
      )}
    </span>
  );
}
