// Products tab read + write hooks. Sourced from the M1 inventory: every
// fetch the legacy products.js makes is mirrored here so the React tab
// can drop the legacy dispatcher without losing functionality.
import { useQuery, useMutation, useQueryClient, type QueryKey } from '@tanstack/react-query';
import { fetchJSON } from '../api/client';
import type {
  Manifest,
  PeerProductGroup,
  Product,
  ProductDependencies,
  ProductHierarchy,
  Idea,
  Comment,
  CommentEnvelope,
} from '../types';

export const productKeys = {
  byPeer: ['products', 'by-peer'] as const,
  product: (id: string) => ['product', id] as const,
  manifests: (id: string) => ['product', id, 'manifests'] as const,
  ideas: (id: string) => ['product', id, 'ideas'] as const,
  deps: (id: string) => ['product', id, 'deps'] as const,
  hierarchy: (id: string) => ['product', id, 'hierarchy'] as const,
  comments: (id: string) => ['product', id, 'comments'] as const,
  search: (q: string) => ['product', 'search', q] as const,
};

const ALL_MANIFESTS_KEY: QueryKey = ['manifests', 'all'];
const ALL_IDEAS_KEY: QueryKey = ['ideas', 'all'];

export function useProductsByPeer() {
  return useQuery({
    queryKey: productKeys.byPeer,
    queryFn: () => fetchJSON<PeerProductGroup[]>('/api/products/by-peer'),
  });
}

export function useProductSearch(q: string, enabled: boolean) {
  return useQuery({
    queryKey: productKeys.search(q),
    queryFn: () => fetchJSON<Product[]>(`/api/products/search?q=${encodeURIComponent(q)}`),
    enabled: enabled && q.length > 0,
  });
}

export function useProduct(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.product(id) : ['product', 'noop'],
    queryFn: () => fetchJSON<Product>(`/api/products/${id}`),
    enabled: Boolean(id),
  });
}

export function useProductManifests(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.manifests(id) : ['product', 'noop', 'manifests'],
    queryFn: () => fetchJSON<Manifest[]>(`/api/products/${id}/manifests`),
    enabled: Boolean(id),
  });
}

export function useProductIdeas(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.ideas(id) : ['product', 'noop', 'ideas'],
    queryFn: () => fetchJSON<Idea[] | null>(`/api/products/${id}/ideas`),
    enabled: Boolean(id),
  });
}

export function useProductDeps(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.deps(id) : ['product', 'noop', 'deps'],
    queryFn: () => fetchJSON<ProductDependencies>(`/api/products/${id}/dependencies?direction=both`),
    enabled: Boolean(id),
  });
}

export function useProductHierarchy(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.hierarchy(id) : ['product', 'noop', 'hierarchy'],
    queryFn: () => fetchJSON<ProductHierarchy>(`/api/products/${id}/hierarchy`),
    enabled: Boolean(id),
  });
}

export function useProductComments(id: string | undefined) {
  return useQuery({
    queryKey: id ? productKeys.comments(id) : ['product', 'noop', 'comments'],
    queryFn: async () => {
      const env = await fetchJSON<CommentEnvelope | Comment[] | null>(`/api/products/${id}/comments`);
      if (!env) return [] as Comment[];
      if (Array.isArray(env)) return env;
      return env.comments ?? [];
    },
    enabled: Boolean(id),
  });
}

export function useAllManifests(enabled: boolean) {
  return useQuery({
    queryKey: ALL_MANIFESTS_KEY,
    queryFn: () => fetchJSON<Manifest[]>('/api/manifests'),
    enabled,
  });
}

export function useAllIdeas(enabled: boolean) {
  return useQuery({
    queryKey: ALL_IDEAS_KEY,
    queryFn: () => fetchJSON<Idea[]>('/api/ideas'),
    enabled,
  });
}

interface CreateProductVars {
  title: string;
  description?: string;
  tags?: string[];
}

export function useCreateProduct() {
  const qc = useQueryClient();
  return useMutation<Product, Error, CreateProductVars>({
    mutationFn: (v) =>
      fetchJSON<Product>('/api/products', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(v),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: productKeys.byPeer });
    },
  });
}

interface UpdateProductVars {
  id: string;
  patch: Partial<Pick<Product, 'title' | 'description' | 'status' | 'tags'>>;
}

export function useUpdateProduct() {
  const qc = useQueryClient();
  return useMutation<Product, Error, UpdateProductVars>({
    mutationFn: ({ id, patch }) =>
      fetchJSON<Product>(`/api/products/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: productKeys.byPeer });
      qc.invalidateQueries({ queryKey: productKeys.product(vars.id) });
    },
  });
}

interface AddDepVars {
  productId: string;
  dependsOnId: string;
}

export function useAddProductDep() {
  const qc = useQueryClient();
  return useMutation<unknown, Error, AddDepVars>({
    mutationFn: ({ productId, dependsOnId }) =>
      fetchJSON(`/api/products/${productId}/dependencies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ depends_on_id: dependsOnId }),
      }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: productKeys.deps(vars.productId) });
      qc.invalidateQueries({ queryKey: productKeys.product(vars.productId) });
    },
  });
}

interface RemoveDepVars {
  productId: string;
  depId: string;
}

export function useRemoveProductDep() {
  const qc = useQueryClient();
  return useMutation<unknown, Error, RemoveDepVars>({
    mutationFn: ({ productId, depId }) =>
      fetchJSON(`/api/products/${productId}/dependencies/${depId}`, { method: 'DELETE' }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: productKeys.deps(vars.productId) });
      qc.invalidateQueries({ queryKey: productKeys.product(vars.productId) });
    },
  });
}

interface LinkManifestVars {
  manifestId: string;
  productId: string | null;
}

export function useLinkManifestToProduct() {
  const qc = useQueryClient();
  return useMutation<Manifest, Error, LinkManifestVars>({
    mutationFn: ({ manifestId, productId }) =>
      fetchJSON<Manifest>(`/api/manifests/${manifestId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ project_id: productId ?? '' }),
      }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: productKeys.byPeer });
      qc.invalidateQueries({ queryKey: ALL_MANIFESTS_KEY });
      if (vars.productId) qc.invalidateQueries({ queryKey: productKeys.manifests(vars.productId) });
    },
  });
}

interface LinkIdeaVars {
  idea: Idea;
  productId: string | null;
}

export function useLinkIdeaToProduct() {
  const qc = useQueryClient();
  return useMutation<Idea, Error, LinkIdeaVars>({
    mutationFn: ({ idea, productId }) =>
      fetchJSON<Idea>(`/api/ideas/${idea.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          project_id: productId ?? '',
          title: idea.title,
          description: idea.description,
          status: idea.status,
          priority: idea.priority,
        }),
      }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ALL_IDEAS_KEY });
      if (vars.productId) qc.invalidateQueries({ queryKey: productKeys.ideas(vars.productId) });
    },
  });
}

interface CreateManifestVars {
  title: string;
  description?: string;
  project_id?: string;
}

export function useCreateManifest() {
  const qc = useQueryClient();
  return useMutation<Manifest, Error, CreateManifestVars>({
    mutationFn: (v) =>
      fetchJSON<Manifest>('/api/manifests', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(v),
      }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: productKeys.byPeer });
      qc.invalidateQueries({ queryKey: ALL_MANIFESTS_KEY });
      if (vars.project_id) qc.invalidateQueries({ queryKey: productKeys.manifests(vars.project_id) });
    },
  });
}
