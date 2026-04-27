import clsx from 'clsx';
import { useUpdateProduct } from '@/lib/queries/products';

const STATUSES = ['draft', 'open', 'closed', 'archive'] as const;
export type ProductStatus = (typeof STATUSES)[number];

export interface StatusPivotsProps {
  productId: string;
  current: string;
}

export function StatusPivots({ productId, current }: StatusPivotsProps) {
  const update = useUpdateProduct();
  return (
    <div className="status-pivots" role="group" aria-label="Product status">
      {STATUSES.map((s) => {
        const active = current === s;
        return (
          <button
            key={s}
            type="button"
            data-status={s}
            disabled={update.isPending}
            aria-pressed={active}
            className={clsx('status-pivot', `status-pivot--${s}`, active && 'is-active')}
            onClick={() => {
              if (!active) update.mutate({ id: productId, status: s });
            }}
          >
            {s}
          </button>
        );
      })}
    </div>
  );
}
