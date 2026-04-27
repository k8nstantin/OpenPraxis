import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchJSON } from '../api';
import type { Idea } from '../types';

export function useProductIdeas(productId: string | undefined) {
  return useQuery({
    queryKey: ['product', productId, 'ideas'],
    queryFn: () => fetchJSON<Idea[]>(`/api/products/${productId}/ideas`),
    enabled: Boolean(productId),
  });
}

export function useAllIdeas() {
  return useQuery({
    queryKey: ['ideas', 'all'],
    queryFn: () => fetchJSON<Idea[]>('/api/ideas'),
  });
}

export function useUpdateIdeaProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      idea,
      productId,
    }: {
      idea: Idea;
      productId: string;
    }) =>
      fetchJSON<Idea>(`/api/ideas/${idea.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        // The legacy endpoint requires the full idea body on PUT.
        body: JSON.stringify({
          project_id: productId,
          title: idea.title,
          description: idea.description ?? '',
          status: idea.status,
          priority: idea.priority,
        }),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['ideas', 'all'] });
      if (vars.productId) {
        qc.invalidateQueries({ queryKey: ['product', vars.productId, 'ideas'] });
        qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
      }
      if (vars.idea.project_id) {
        qc.invalidateQueries({ queryKey: ['product', vars.idea.project_id, 'ideas'] });
      }
    },
  });
}
