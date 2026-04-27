import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchJSON } from '../api';
import type { Manifest } from '../types';

export function useAllManifests() {
  return useQuery({
    queryKey: ['manifests', 'all'],
    queryFn: () => fetchJSON<Manifest[]>('/api/manifests'),
  });
}

export interface CreateManifestInput {
  title: string;
  description?: string;
  project_id?: string;
}

export function useCreateManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: CreateManifestInput) =>
      fetchJSON<Manifest>('/api/manifests', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(vars),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['manifests', 'all'] });
      if (vars.project_id) {
        qc.invalidateQueries({ queryKey: ['product', vars.project_id, 'manifests'] });
        qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
      }
    },
  });
}

export function useUpdateManifestProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ manifestId, productId }: { manifestId: string; productId: string }) =>
      fetchJSON<Manifest>(`/api/manifests/${manifestId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ project_id: productId }),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['manifests', 'all'] });
      qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
      if (vars.productId) {
        qc.invalidateQueries({ queryKey: ['product', vars.productId, 'manifests'] });
      }
    },
  });
}
