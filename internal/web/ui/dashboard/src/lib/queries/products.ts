import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchJSON } from '../api';
import type {
  Manifest,
  PeerProductGroup,
  Product,
  ProductDependencies,
  ProductHierarchy,
} from '../types';

export function useProductsByPeer() {
  return useQuery({
    queryKey: ['products', 'by-peer'],
    queryFn: () => fetchJSON<PeerProductGroup[]>('/api/products/by-peer'),
  });
}

export function useProduct(id: string | undefined) {
  return useQuery({
    queryKey: ['product', id],
    queryFn: () => fetchJSON<Product>(`/api/products/${id}`),
    enabled: Boolean(id),
  });
}

export function useProductManifests(id: string | undefined) {
  return useQuery({
    queryKey: ['product', id, 'manifests'],
    queryFn: () => fetchJSON<Manifest[]>(`/api/products/${id}/manifests`),
    enabled: Boolean(id),
  });
}

export function useProductHierarchy(id: string | undefined) {
  return useQuery({
    queryKey: ['product', id, 'hierarchy'],
    queryFn: () => fetchJSON<ProductHierarchy>(`/api/products/${id}/hierarchy`),
    enabled: Boolean(id),
  });
}

export function useProductDependencies(id: string | undefined) {
  return useQuery({
    queryKey: ['product', id, 'dependencies'],
    queryFn: () =>
      fetchJSON<ProductDependencies>(
        `/api/products/${id}/dependencies?direction=both`,
      ),
    enabled: Boolean(id),
  });
}

export function useAllProductsFlat() {
  const groups = useProductsByPeer();
  const flat: Product[] = (groups.data ?? []).flatMap((g) => g.products ?? []);
  return { ...groups, data: flat };
}

export interface CreateProductInput {
  title: string;
  description?: string;
  tags?: string[];
}

export function useCreateProduct() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: CreateProductInput) =>
      fetchJSON<Product>('/api/products', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(vars),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
    },
  });
}

export interface UpdateProductInput {
  id: string;
  title?: string;
  description?: string;
  tags?: string[];
  status?: string;
}

export function useUpdateProduct() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...rest }: UpdateProductInput) =>
      fetchJSON<Product>(`/api/products/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rest),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
      qc.invalidateQueries({ queryKey: ['product', vars.id] });
    },
  });
}

export function useAddProductDependency(productId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (depId: string) =>
      fetchJSON<unknown>(`/api/products/${productId}/dependencies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ depends_on_id: depId }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['product', productId, 'dependencies'] });
      qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
    },
  });
}

export function useRemoveProductDependency(productId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (depId: string) =>
      fetchJSON<unknown>(
        `/api/products/${productId}/dependencies/${depId}`,
        { method: 'DELETE' },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['product', productId, 'dependencies'] });
      qc.invalidateQueries({ queryKey: ['products', 'by-peer'] });
    },
  });
}

export function useProductSearch() {
  return async (query: string): Promise<Product[]> => {
    if (!query.trim()) return [];
    return fetchJSON<Product[]>(
      `/api/products/search?q=${encodeURIComponent(query)}`,
      { silent: true },
    );
  };
}
