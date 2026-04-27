import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchJSON } from '../api';
import type { Comment } from '../types';

export type CommentScope = 'products' | 'manifests' | 'tasks' | 'ideas';

export function useComments(scope: CommentScope, id: string | undefined) {
  return useQuery({
    queryKey: ['comments', scope, id],
    queryFn: () => fetchJSON<Comment[]>(`/api/${scope}/${id}/comments`),
    enabled: Boolean(id),
  });
}

export function useAddComment(scope: CommentScope, id: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      fetchJSON<Comment>(`/api/${scope}/${id}/comments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ author: 'operator', type: 'user_note', body }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['comments', scope, id] });
    },
  });
}
