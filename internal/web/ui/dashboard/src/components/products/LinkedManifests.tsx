import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import {
  useProductManifests,
} from '@/lib/queries/products';
import { useUpdateManifestProject } from '@/lib/queries/manifests';
import type { Manifest } from '@/lib/types';
import { LinkManifestPicker } from './LinkManifestPicker';
import { NewManifestDialog } from './NewManifestDialog';

export interface LinkedManifestsProps {
  productId: string;
  productTitle: string;
}

export function LinkedManifests({ productId, productTitle }: LinkedManifestsProps) {
  const q = useProductManifests(productId);
  const unlink = useUpdateManifestProject();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const navigate = useNavigate();

  return (
    <section className="linked-manifests" aria-labelledby="linked-manifests-title">
      <header className="linked-manifests__header">
        <h3 id="linked-manifests-title">
          Linked Manifests {q.data ? <span className="sub-count">({q.data.length})</span> : null}
        </h3>
        <div className="linked-manifests__actions">
          <Button size="sm" variant="ghost" onClick={() => setNewOpen(true)}>
            + New Manifest
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setPickerOpen(true)}>
            + Link Manifest
          </Button>
        </div>
      </header>
      {q.isLoading && <EmptyState message="Loading…" />}
      {q.data && q.data.length === 0 && <EmptyState message="No manifests linked yet." />}
      {q.data && q.data.length > 0 && (
        <ul className="linked-manifests__list" role="list">
          {q.data.map((m) => (
            <li key={m.id}>
              <ManifestRow
                manifest={m}
                onOpen={() => navigate(`/manifests/${m.id}`)}
                onUnlink={() => {
                  if (window.confirm(`Unlink manifest "${m.title}" from product?`)) {
                    unlink.mutate({ manifestId: m.id, productId: '' });
                  }
                }}
                disabled={unlink.isPending}
              />
            </li>
          ))}
        </ul>
      )}
      <LinkManifestPicker productId={productId} open={pickerOpen} onOpenChange={setPickerOpen} />
      <NewManifestDialog
        productId={productId}
        productTitle={productTitle}
        open={newOpen}
        onOpenChange={setNewOpen}
      />
    </section>
  );
}

function ManifestRow({
  manifest,
  onOpen,
  onUnlink,
  disabled,
}: {
  manifest: Manifest;
  onOpen: () => void;
  onUnlink: () => void;
  disabled: boolean;
}) {
  const m = manifest;
  return (
    <div className="manifest-row">
      <button
        type="button"
        className="manifest-row__main"
        onClick={onOpen}
        aria-label={`Open manifest ${m.title}`}
      >
        <span className="marker">{m.marker}</span>
        <Badge tone={m.status === 'open' ? 'success' : m.status === 'closed' ? 'neutral' : 'info'}>
          {m.status}
        </Badge>
        <span className="manifest-title">{m.title}</span>
        <span className="manifest-row__meta">
          {(m.total_tasks ?? 0) > 0 && <span>{m.total_tasks} tasks</span>}
          {(m.total_turns ?? 0) > 0 && <span>{m.total_turns} turns</span>}
          {(m.total_cost ?? 0) > 0 && <span className="cost">${(m.total_cost ?? 0).toFixed(2)}</span>}
        </span>
      </button>
      <button
        type="button"
        className="manifest-row__rm"
        onClick={(e) => {
          e.stopPropagation();
          onUnlink();
        }}
        disabled={disabled}
        aria-label={`Unlink ${m.title} from product`}
      >
        ×
      </button>
    </div>
  );
}
