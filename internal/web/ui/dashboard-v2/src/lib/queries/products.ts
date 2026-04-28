import { useQuery } from '@tanstack/react-query'
import type {
  Comment,
  HierarchyNode,
  Idea,
  Manifest,
  Product,
  ProductDependency,
} from '@/lib/types'

// react-query hooks for the Products tab. Read-only for the first
// iteration. Write mutations land in a follow-up alongside the
// dependency editor and the create/edit dialogs.

export const productKeys = {
  all: ['products'] as const,
  list: () => [...productKeys.all, 'list'] as const,
  detail: (id: string) => [...productKeys.all, 'detail', id] as const,
  manifests: (id: string) => [...productKeys.all, 'manifests', id] as const,
  ideas: (id: string) => [...productKeys.all, 'ideas', id] as const,
  comments: (id: string) => [...productKeys.all, 'comments', id] as const,
  deps: (id: string) => [...productKeys.all, 'deps', id] as const,
  hierarchy: (id: string) => [...productKeys.all, 'hierarchy', id] as const,
  descriptionHistory: (id: string) =>
    [...productKeys.all, 'description-history', id] as const,
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

export function useProducts() {
  return useQuery({
    queryKey: productKeys.list(),
    queryFn: () => fetchJSON<Product[]>('/api/products'),
    staleTime: 30 * 1000,
  })
}

export function useProduct(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.detail(id ?? ''),
    queryFn: () => fetchJSON<Product>(`/api/products/${id}`),
    enabled: !!id,
    staleTime: 15 * 1000,
  })
}

export function useProductHierarchy(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.hierarchy(id ?? ''),
    queryFn: () => fetchJSON<HierarchyNode>(`/api/products/${id}/hierarchy`),
    enabled: !!id,
    staleTime: 30 * 1000,
  })
}

export function useProductManifests(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.manifests(id ?? ''),
    queryFn: () => fetchJSON<Manifest[]>(`/api/products/${id}/manifests`),
    enabled: !!id,
    staleTime: 15 * 1000,
  })
}

export function useProductIdeas(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.ideas(id ?? ''),
    queryFn: () => fetchJSON<Idea[]>(`/api/products/${id}/ideas`),
    enabled: !!id,
    staleTime: 30 * 1000,
  })
}

// Comments endpoint returns either `Comment[]` directly or wrapped in
// `{ comments: [...] }` depending on legacy code paths; normalize both.
export function useProductComments(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.comments(id ?? ''),
    queryFn: async () => {
      const res = await fetch(`/api/products/${id}/comments`)
      if (!res.ok) throw new Error(`comments → ${res.status}`)
      const data = (await res.json()) as Comment[] | { comments: Comment[] }
      return Array.isArray(data) ? data : (data.comments ?? [])
    },
    enabled: !!id,
    staleTime: 15 * 1000,
  })
}

export function useProductDependencies(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.deps(id ?? ''),
    queryFn: () =>
      fetchJSON<ProductDependency[]>(`/api/products/${id}/dependencies`),
    enabled: !!id,
    staleTime: 30 * 1000,
  })
}

// Description revision history is a sub-set of the comments stream
// filtered to type=description_revision. The Portal A endpoint
// `/api/descriptions/<entity>/<id>/history` returns the revision rows
// directly; if that's not exposed yet on the V2 path, callers can
// derive it client-side from the comments query.
export function useProductDescriptionHistory(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.descriptionHistory(id ?? ''),
    queryFn: async () => {
      const res = await fetch(`/api/descriptions/product/${id}/history`)
      if (!res.ok) {
        // Fallback: derive from comments. Caller filters where needed.
        return [] as Comment[]
      }
      return (await res.json()) as Comment[]
    },
    enabled: !!id,
    staleTime: 60 * 1000,
  })
}
