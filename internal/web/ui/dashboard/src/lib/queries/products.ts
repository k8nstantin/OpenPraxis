import { useQuery } from '@tanstack/react-query';
import { fetchJSON } from '../api';
import type { Manifest, PeerProductGroup, Product, ProductHierarchy } from '../types';

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
