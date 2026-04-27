import { useMemo, useState, type ReactNode } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { EmptyState } from '@/components/ui/EmptyState';

export interface PickerDialogItem {
  id: string;
  search: string;
  render: ReactNode;
}

export interface PickerDialogProps {
  open: boolean;
  title: ReactNode;
  description?: ReactNode;
  items: PickerDialogItem[];
  busy?: boolean;
  emptyMessage?: string;
  onOpenChange: (open: boolean) => void;
  onPick: (id: string) => void;
}

/**
 * Simple modal list picker. Used by Products M1 for "+ Link Manifest",
 * "+ Link Idea", and "+ Depends On". Filters by case-insensitive substring
 * over the item's `search` blob. Click-outside / Esc dismisses via the
 * Dialog primitive (Radix).
 */
export function PickerDialog({
  open,
  title,
  description,
  items,
  busy,
  emptyMessage = 'Nothing available to link.',
  onOpenChange,
  onPick,
}: PickerDialogProps) {
  const [filter, setFilter] = useState('');
  const filtered = useMemo(() => {
    const f = filter.trim().toLowerCase();
    if (!f) return items;
    return items.filter((it) => it.search.toLowerCase().includes(f));
  }, [filter, items]);

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) setFilter('');
      }}
      title={title}
      description={description}
      size="md"
    >
      <input
        type="search"
        className="ui-input"
        placeholder="Filter…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        aria-label="Filter list"
        autoFocus
      />
      <ul className="picker-list" role="listbox" aria-label="Available items">
        {filtered.length === 0 && (
          <li className="picker-list__empty">
            <EmptyState message={items.length === 0 ? emptyMessage : 'No items match the filter.'} />
          </li>
        )}
        {filtered.map((it) => (
          <li key={it.id} className="picker-list__item">
            <button
              type="button"
              className="picker-list__btn"
              disabled={busy}
              onClick={() => onPick(it.id)}
            >
              {it.render}
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}
