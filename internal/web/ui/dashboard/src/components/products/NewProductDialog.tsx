import { useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { FormActions, FormError, FormField } from '@/components/forms';
import {
  useAddProductDependency,
  useCreateProduct,
} from '@/lib/queries/products';
import type { Product } from '@/lib/types';

interface BaseProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (p: Product) => void;
}

export interface NewProductDialogProps extends BaseProps {
  parent?: { id: string; title: string };
}

export function NewProductDialog({ open, onOpenChange, onCreated, parent }: NewProductDialogProps) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [tags, setTags] = useState('');
  const [error, setError] = useState<string | null>(null);
  const create = useCreateProduct();
  const addDep = useAddProductDependency(parent?.id);

  const reset = () => {
    setTitle('');
    setDescription('');
    setTags('');
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
      {
        title: t,
        description: description.trim(),
        tags: tags
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
      },
      {
        onSuccess: (p) => {
          // Sub-product flow: wire the new product as a dependency edge
          // from the parent so it appears nested in the tree (mirrors how
          // the legacy umbrella → sub-product relationship works).
          if (parent && p?.id) {
            addDep.mutate(p.id, {
              onSuccess: () => {
                onCreated?.(p);
                close();
              },
              onError: (err) => setError(err.message),
            });
          } else {
            onCreated?.(p);
            close();
          }
        },
        onError: (err) => setError(err.message),
      },
    );
  };

  const busy = create.isPending || addDep.isPending;
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => (o ? onOpenChange(true) : close())}
      title={parent ? `New Sub-Product under ${parent.title}` : 'New Product'}
      description={
        parent
          ? 'Creates a new product and links it as a dependency of the current product.'
          : 'Group your manifests under a top-level product.'
      }
      footer={null}
    >
      <form onSubmit={onSubmit} aria-label={parent ? 'New sub-product form' : 'New product form'}>
        <FormField label="Title" htmlFor="new-product-title" required>
          <input
            id="new-product-title"
            className="ui-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Product name"
            autoFocus
            required
          />
        </FormField>
        <FormField label="Description" htmlFor="new-product-desc">
          <textarea
            id="new-product-desc"
            className="ui-input ui-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What is this product/project about?"
            rows={4}
          />
        </FormField>
        <FormField label="Tags (comma-separated)" htmlFor="new-product-tags">
          <input
            id="new-product-tags"
            className="ui-input"
            value={tags}
            onChange={(e) => setTags(e.target.value)}
            placeholder="e.g. openpraxis, backend"
          />
        </FormField>
        {error && <FormError>{error}</FormError>}
        <FormActions>
          <Button type="button" variant="ghost" onClick={close} disabled={busy}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy}>
            {parent ? 'Create Sub-Product' : 'Create Product'}
          </Button>
        </FormActions>
      </form>
    </Dialog>
  );
}
