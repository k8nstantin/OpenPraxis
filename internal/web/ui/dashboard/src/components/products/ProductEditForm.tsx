import { useState } from 'react';
import { Button } from '@/components/ui/Button';
import { FormActions, FormError, FormField } from '@/components/forms';
import { useUpdateProduct } from '@/lib/queries/products';
import type { Product } from '@/lib/types';

export interface ProductEditFormProps {
  product: Product;
  onCancel: () => void;
  onSaved: () => void;
}

function tagsToString(tags: string[] | undefined): string {
  return (tags ?? []).join(', ');
}

function parseTags(raw: string): string[] {
  return raw
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean);
}

export function ProductEditForm({ product, onCancel, onSaved }: ProductEditFormProps) {
  const [title, setTitle] = useState(product.title);
  const [description, setDescription] = useState(product.description ?? '');
  const [tags, setTags] = useState(tagsToString(product.tags));
  const [error, setError] = useState<string | null>(null);
  const update = useUpdateProduct();

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError('Title is required');
      return;
    }
    setError(null);
    update.mutate(
      { id: product.id, title: title.trim(), description, tags: parseTags(tags) },
      { onSuccess: onSaved, onError: (err) => setError(err.message) },
    );
  };

  return (
    <form className="product-edit-form" onSubmit={onSubmit} aria-label="Edit product">
      <FormField label="Title" htmlFor="edit-product-title" required error={!title.trim() && error ? error : undefined}>
        <input
          id="edit-product-title"
          className="ui-input"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
      </FormField>
      <FormField label="Description" htmlFor="edit-product-desc" hint="Markdown supported">
        <textarea
          id="edit-product-desc"
          className="ui-input ui-textarea"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={8}
        />
      </FormField>
      <FormField label="Tags (comma-separated)" htmlFor="edit-product-tags">
        <input
          id="edit-product-tags"
          className="ui-input"
          value={tags}
          onChange={(e) => setTags(e.target.value)}
        />
      </FormField>
      {error && title.trim() && <FormError id="edit-product-error">{error}</FormError>}
      <FormActions>
        <Button type="button" variant="ghost" onClick={onCancel} disabled={update.isPending}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" loading={update.isPending}>
          Save
        </Button>
      </FormActions>
    </form>
  );
}
