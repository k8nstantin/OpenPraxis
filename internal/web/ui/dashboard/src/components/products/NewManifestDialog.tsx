import { useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { FormActions, FormError, FormField } from '@/components/forms';
import { useCreateManifest } from '@/lib/queries/manifests';
import type { Manifest } from '@/lib/types';

export interface NewManifestDialogProps {
  open: boolean;
  productId: string;
  productTitle: string;
  onOpenChange: (open: boolean) => void;
  onCreated?: (m: Manifest) => void;
}

export function NewManifestDialog({
  open,
  productId,
  productTitle,
  onOpenChange,
  onCreated,
}: NewManifestDialogProps) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);
  const create = useCreateManifest();

  const reset = () => {
    setTitle('');
    setDescription('');
    setError(null);
  };
  const close = () => {
    reset();
    onOpenChange(false);
  };

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const t = title.trim();
    if (!t) {
      setError('Title is required');
      return;
    }
    setError(null);
    create.mutate(
      { title: t, description: description.trim(), project_id: productId },
      {
        onSuccess: (m) => {
          onCreated?.(m);
          close();
        },
        onError: (err) => setError(err.message),
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => (o ? onOpenChange(true) : close())}
      title={`New Manifest under ${productTitle}`}
      description="Manifest will be pre-linked to this product."
    >
      <form onSubmit={onSubmit} aria-label="New manifest form">
        <FormField label="Title" htmlFor="new-manifest-title" required>
          <input
            id="new-manifest-title"
            className="ui-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Manifest title"
            autoFocus
            required
          />
        </FormField>
        <FormField label="Description" htmlFor="new-manifest-desc">
          <textarea
            id="new-manifest-desc"
            className="ui-input ui-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Spec / scope / acceptance"
            rows={6}
          />
        </FormField>
        {error && <FormError>{error}</FormError>}
        <FormActions>
          <Button type="button" variant="ghost" onClick={close} disabled={create.isPending}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            Create Manifest
          </Button>
        </FormActions>
      </form>
    </Dialog>
  );
}
