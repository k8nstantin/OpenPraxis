import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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

// /api/products/{id}/dependencies returns `{ deps: [...] }` (wrapped),
// not a bare array. Normalize so callers always see ProductDependency[].
// (Once the endpoint shape is stabilized + tygo-typed we can drop the
// unwrap; for now the runtime check costs nothing and survives a future
// schema flip in either direction.)
export function useProductDependencies(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.deps(id ?? ''),
    queryFn: async () => {
      const res = await fetch(`/api/products/${id}/dependencies`)
      if (!res.ok) throw new Error(`dependencies → ${res.status}`)
      const data = (await res.json()) as
        | ProductDependency[]
        | { deps: ProductDependency[] }
      return Array.isArray(data) ? data : (data.deps ?? [])
    },
    enabled: !!id,
    staleTime: 30 * 1000,
  })
}

// Description revision row returned by /api/products/{id}/description/history.
// Distinct from Comment — has version + body but not type/target.
export interface DescriptionRevision {
  id: string
  version: number
  author: string
  body: string
  created_at: number | string
  created_at_iso?: string
}

// GET /api/products/{id}/description/history → `{ items: [...] }`.
// Earlier draft hit `/api/descriptions/product/<id>/history` which 404s.
// Correct base path is `/api/{scope}/{id}/description/...` (handlers_
// description.go line 181). Response is wrapped in `items`.
export function useProductDescriptionHistory(id: string | undefined) {
  return useQuery({
    queryKey: productKeys.descriptionHistory(id ?? ''),
    queryFn: async () => {
      const res = await fetch(`/api/products/${id}/description/history`)
      if (!res.ok) throw new Error(`description history → ${res.status}`)
      const data = (await res.json()) as
        | { items: DescriptionRevision[] }
        | DescriptionRevision[]
      return Array.isArray(data) ? data : (data.items ?? [])
    },
    enabled: !!id,
    staleTime: 60 * 1000,
  })
}

// PUT /api/products/{id} — update fields. Used by the Description
// edit save path and any future field-edit affordance.
export function useUpdateProduct(id: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (patch: Partial<Product>) => {
      const res = await fetch(`/api/products/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch),
      })
      if (!res.ok) throw new Error(`update product → ${res.status}`)
      return (await res.json()) as Product
    },
    onSuccess: () => {
      // Invalidate detail + list so the new title / description / status
      // shows on next render. Hierarchy + comments aren't affected here
      // (the description edit creates a description_revision comment
      // server-side; that invalidation is in useCreateProductComment).
      if (id) qc.invalidateQueries({ queryKey: productKeys.detail(id) })
      qc.invalidateQueries({ queryKey: productKeys.list() })
      if (id) qc.invalidateQueries({ queryKey: productKeys.comments(id) })
      if (id)
        qc.invalidateQueries({ queryKey: productKeys.descriptionHistory(id) })
    },
  })
}

// POST /api/products/{id}/comments — compose a new comment. Used by
// the Comments tab composer.
export function useCreateProductComment(id: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      author: string
      type: string
      body: string
    }) => {
      const res = await fetch(`/api/products/${id}/comments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw new Error(`post comment → ${res.status}`)
      return (await res.json()) as Comment
    },
    onSuccess: () => {
      if (id) qc.invalidateQueries({ queryKey: productKeys.comments(id) })
    },
  })
}

// ── Dependency mutations ─────────────────────────────────────────────
//
// Three edge directions managed from the Dependencies editor:
//
//   1. Upstream product   — THIS product depends on X
//      POST   /api/products/{this}/dependencies/{X}
//      DELETE /api/products/{this}/dependencies/{X}
//
//   2. Downstream product — X depends on THIS (X becomes a sub-product)
//      POST   /api/products/{X}/dependencies/{this}
//      DELETE /api/products/{X}/dependencies/{this}
//
//   3. Owned manifest     — re-parent manifest.project_id = THIS
//      PUT /api/manifests/{m} with body { project_id: this | "" }
//
// Every mutation posts a `dependency_revision` agent_note comment on
// THIS product with an op + snapshot so the revision history can show
// what changed. Same versioning rhythm as description revisions.

interface DepRevisionPayload {
  op: 'add' | 'remove'
  kind: 'product_upstream' | 'product_downstream' | 'manifest'
  target: { id: string; marker: string; title: string }
  snapshot: {
    upstream: string[]
    downstream: string[]
    manifests: string[]
  }
}

async function postDepRevision(
  productId: string,
  payload: DepRevisionPayload
): Promise<void> {
  const body = JSON.stringify(payload, null, 2)
  await fetch(`/api/products/${productId}/comments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      author: 'operator',
      // Use existing `agent_note` type with a structured body. A
      // dedicated `dependency_revision` type can be added to the
      // backend registry later — for now agent_note keeps the comment
      // valid + visible in the existing revisions stream filter.
      type: 'agent_note',
      body: '<dependency_revision>\n' + body + '\n</dependency_revision>',
    }),
  })
}

function invalidateDepCaches(qc: ReturnType<typeof useQueryClient>, id: string) {
  qc.invalidateQueries({ queryKey: productKeys.deps(id) })
  qc.invalidateQueries({ queryKey: productKeys.manifests(id) })
  qc.invalidateQueries({ queryKey: productKeys.hierarchy(id) })
  qc.invalidateQueries({ queryKey: productKeys.comments(id) })
}

// 1. Upstream — this depends on X.
export function useAddUpstreamProductDep(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `/api/products/${productId}/dependencies/${input.target.id}`,
        { method: 'POST' }
      )
      if (!res.ok && res.status !== 409)
        throw new Error(`add upstream → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'add',
          kind: 'product_upstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

export function useRemoveUpstreamProductDep(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `/api/products/${productId}/dependencies/${input.target.id}`,
        { method: 'DELETE' }
      )
      if (!res.ok && res.status !== 404)
        throw new Error(`remove upstream → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'remove',
          kind: 'product_upstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

// 2. Downstream — X depends on this. Edit goes against X's deps API.
export function useAddDownstreamProductDep(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `/api/products/${input.target.id}/dependencies/${productId}`,
        { method: 'POST' }
      )
      if (!res.ok && res.status !== 409)
        throw new Error(`add downstream → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'add',
          kind: 'product_downstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

export function useRemoveDownstreamProductDep(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `/api/products/${input.target.id}/dependencies/${productId}`,
        { method: 'DELETE' }
      )
      if (!res.ok && res.status !== 404)
        throw new Error(`remove downstream → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'remove',
          kind: 'product_downstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

// 3. Manifest — re-parent / unlink via manifest.project_id.
export function useLinkManifest(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(`/api/manifests/${input.target.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ project_id: productId }),
      })
      if (!res.ok) throw new Error(`link manifest → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'add',
          kind: 'manifest',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

export function useUnlinkManifest(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(`/api/manifests/${input.target.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ project_id: '' }),
      })
      if (!res.ok) throw new Error(`unlink manifest → ${res.status}`)
      if (productId)
        await postDepRevision(productId, {
          op: 'remove',
          kind: 'manifest',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}

// All-manifests query — used by the manifest picker. Filtered client-
// side to "manifests not already linked to this product."
export function useAllManifests() {
  return useQuery({
    queryKey: ['manifests', 'list'],
    queryFn: async () => {
      const res = await fetch('/api/manifests')
      if (!res.ok) throw new Error(`manifests → ${res.status}`)
      return (await res.json()) as Manifest[]
    },
    staleTime: 30 * 1000,
  })
}

// Restore — apply a prior snapshot as the current dependency state.
//
// Diff vs current:
//   to-add    = snapshot.X − current.X     (re-add what was removed since)
//   to-remove = current.X − snapshot.X     (remove what was added since)
//
// Fire add/remove ops in sequence. Skip 404/409 (idempotent — target
// already gone or already linked). After the diff is applied, log a
// new dependency_revision capturing the restore op + the resulting
// state so the history reads "Restored to <timestamp>" forward in time.
export function useRestoreDependencySnapshot(
  productId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      snapshot: { downstream: string[]; manifests: string[] }
      revisionLabel: string
      currentDownstream: { id: string; marker: string; title: string }[]
      currentManifests: { id: string; marker: string; title: string }[]
    }) => {
      if (!productId) return
      const targetSubs = new Set(input.snapshot.downstream)
      const targetManifests = new Set(input.snapshot.manifests)
      const currentSubs = new Set(
        input.currentDownstream.map((r) => r.id)
      )
      const currentManifestSet = new Set(
        input.currentManifests.map((r) => r.id)
      )

      const subsToAdd = [...targetSubs].filter((id) => !currentSubs.has(id))
      const subsToRemove = [...currentSubs].filter(
        (id) => !targetSubs.has(id)
      )
      const manifestsToAdd = [...targetManifests].filter(
        (id) => !currentManifestSet.has(id)
      )
      const manifestsToRemove = [...currentManifestSet].filter(
        (id) => !targetManifests.has(id)
      )

      // Sub-product re-parents (downstream — X depends on this).
      for (const subId of subsToAdd) {
        const r = await fetch(
          `/api/products/${subId}/dependencies/${productId}`,
          { method: 'POST' }
        )
        if (!r.ok && r.status !== 409) {
          throw new Error(`restore: add sub ${subId} → ${r.status}`)
        }
      }
      for (const subId of subsToRemove) {
        const r = await fetch(
          `/api/products/${subId}/dependencies/${productId}`,
          { method: 'DELETE' }
        )
        if (!r.ok && r.status !== 404) {
          throw new Error(`restore: remove sub ${subId} → ${r.status}`)
        }
      }

      // Manifest re-parents.
      for (const mId of manifestsToAdd) {
        const r = await fetch(`/api/manifests/${mId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ project_id: productId }),
        })
        if (!r.ok)
          throw new Error(`restore: link manifest ${mId} → ${r.status}`)
      }
      for (const mId of manifestsToRemove) {
        const r = await fetch(`/api/manifests/${mId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ project_id: '' }),
        })
        if (!r.ok)
          throw new Error(`restore: unlink manifest ${mId} → ${r.status}`)
      }

      // Log the restore as a new dependency_revision so the history
      // continues forward and a future restore can target this point.
      const restoreBody =
        '<dependency_revision>\n' +
        JSON.stringify(
          {
            op: 'restore',
            kind: 'snapshot',
            target: { id: '', marker: '', title: input.revisionLabel },
            snapshot: {
              upstream: [],
              downstream: [...targetSubs],
              manifests: [...targetManifests],
            },
          },
          null,
          2
        ) +
        '\n</dependency_revision>'
      await fetch(`/api/products/${productId}/comments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          author: 'operator',
          type: 'agent_note',
          body: restoreBody,
        }),
      })

      return {
        addedSubs: subsToAdd.length,
        removedSubs: subsToRemove.length,
        addedManifests: manifestsToAdd.length,
        removedManifests: manifestsToRemove.length,
      }
    },
    onSuccess: () => productId && invalidateDepCaches(qc, productId),
  })
}
