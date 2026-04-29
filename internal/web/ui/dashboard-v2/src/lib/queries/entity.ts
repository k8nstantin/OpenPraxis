import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  Comment,
  HierarchyNode,
  Manifest,
  Product,
  ProductDependency,
  Task,
} from '@/lib/types'

// Generic entity queries layer for Products + Manifests + Tasks.
//
// All three kinds share the same 5-tab surface; the only thing that
// changes is the URL prefix and the children/hierarchy semantics. Each
// hook takes a `kind` and dispatches to the correct API path. See
// features/entity/* for the React surface that consumes these.
//
// Path conventions (verified against handlers_*.go in this PR):
//   /api/products/{id}                      GET, PUT
//   /api/products/{id}/manifests            GET (children)
//   /api/products/{id}/hierarchy            GET (recursive descent)
//   /api/products/{id}/dependencies         GET (sub-products), POST (body-style)
//   /api/products/{id}/dependencies/{X}     DELETE
//   /api/products/{id}/comments             GET, POST
//   /api/products/{id}/description/history  GET
//   /api/products/{id}/settings             GET, PUT, DELETE/{key}
//   /api/manifests/{id}                     GET, PUT
//   /api/manifests/{id}/tasks               GET (children)
//   /api/manifests/{id}/dependencies        GET (?direction=out), POST (body-style)
//   /api/manifests/{id}/dependencies/{X}    DELETE
//   /api/manifests/{id}/comments            GET, POST
//   /api/manifests/{id}/description/history GET
//   /api/manifests/{id}/settings            GET, PUT, DELETE/{key}
//   /api/tasks/{id}                         GET, PATCH
//   /api/tasks/{id}/dependencies            GET (?direction=out), POST (body-style)
//   /api/tasks/{id}/dependencies/{X}        DELETE
//   /api/tasks/{id}/comments                GET, POST
//   /api/tasks/{id}/description/history     GET
//   /api/tasks/{id}/settings                GET, PUT, DELETE/{key}
//
// Tasks are leaves in the product → manifest → task tree:
//   - useEntityChildren: returns []  (tasks have no children)
//   - useEntityHierarchy: disabled    (no recursive descent)

export type EntityKind = 'product' | 'manifest' | 'task'

// EntityRecord is the union of fields that the generic surface reads.
// Cast to Product / Manifest / Task as needed for kind-specific extras.
export type EntityRecord = Product | Manifest | Task

export const entityKeys = {
  all: (kind: EntityKind) => [kind] as const,
  list: (kind: EntityKind) => [...entityKeys.all(kind), 'list'] as const,
  detail: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'detail', id] as const,
  children: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'children', id] as const,
  comments: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'comments', id] as const,
  deps: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'deps', id] as const,
  hierarchy: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'hierarchy', id] as const,
  descriptionHistory: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'description-history', id] as const,
}

function basePath(kind: EntityKind): string {
  if (kind === 'product') return '/api/products'
  if (kind === 'task') return '/api/tasks'
  return '/api/manifests'
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

// ── Reads ─────────────────────────────────────────────────────────────

// Graph query — flat (nodes, edges) shape over the unified
// relationships SCD-2 table. Single canonical source for the DAG tab;
// no hierarchy-walker games, no kind-specific endpoints. Backend:
// GET /api/relationships/graph?root_id=&root_kind=&depth=&edge_kinds=
export interface GraphNode {
  id: string
  kind: 'product' | 'manifest' | 'task'
  title: string
  status: string
}
export interface GraphEdge {
  id: string
  source: string
  target: string
  kind: 'owns' | 'depends_on' | 'reviews' | 'links_to'
}
export interface GraphPayload {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export function useEntityGraph(
  kind: EntityKind,
  id: string | undefined,
  depth = 10
) {
  return useQuery({
    queryKey: [...entityKeys.all(kind), 'graph', id ?? '', depth] as const,
    queryFn: async () => {
      const url = `/api/relationships/graph?root_id=${id}&root_kind=${kind}&depth=${depth}`
      const res = await fetch(url)
      if (!res.ok) throw new Error(`graph → ${res.status}`)
      return (await res.json()) as GraphPayload
    },
    enabled: !!id,
    staleTime: 30 * 1000,
  })
}

export function useEntityList(kind: EntityKind) {
  return useQuery({
    queryKey: entityKeys.list(kind),
    queryFn: () => fetchJSON<EntityRecord[]>(basePath(kind)),
    staleTime: 30 * 1000,
  })
}

export function useEntity(kind: EntityKind, id: string | undefined) {
  return useQuery({
    queryKey: entityKeys.detail(kind, id ?? ''),
    queryFn: () => fetchJSON<EntityRecord>(`${basePath(kind)}/${id}`),
    enabled: !!id,
    staleTime: 15 * 1000,
  })
}

// Hierarchy is products-only — manifests don't expose recursive
// descendants. For manifests we synthesize a minimal hierarchy from the
// detail row + children + deps in the DAG tab; this hook just returns
// undefined for kind === 'manifest'.
export function useEntityHierarchy(
  kind: EntityKind,
  id: string | undefined
) {
  return useQuery({
    queryKey: entityKeys.hierarchy(kind, id ?? ''),
    queryFn: () =>
      fetchJSON<HierarchyNode>(`${basePath(kind)}/${id}/hierarchy`),
    enabled: kind === 'product' && !!id,
    staleTime: 30 * 1000,
  })
}

// Children — products → manifests; manifests → tasks; tasks → none.
// Same shape on the wire ({id, marker, title, status} subset), so the
// consumers (Dependencies tab + DAG tab) can treat the rows uniformly.
// Tasks are leaves: the query is disabled and resolves to an empty
// array so callers don't have to special-case undefined.
export function useEntityChildren(
  kind: EntityKind,
  id: string | undefined
) {
  const path =
    kind === 'product'
      ? `/api/products/${id}/manifests`
      : kind === 'manifest'
        ? `/api/manifests/${id}/tasks`
        : ''
  return useQuery({
    queryKey: entityKeys.children(kind, id ?? ''),
    queryFn: () =>
      kind === 'task'
        ? Promise.resolve([] as unknown[])
        : fetchJSON<Manifest[] | unknown[]>(path),
    enabled: !!id && kind !== 'task',
    staleTime: 15 * 1000,
  })
}

// Dependency rows — same {id, marker, title, status} shape. Manifest's
// endpoint wraps in `{deps: [...]}` AND requires direction=out; the
// product's wraps in `{deps: [...]}` too; the task's mirrors manifest
// and returns 0-or-1 rows because tasks carry at most one dep on the
// underlying tasks.depends_on column. Normalize all three to a bare
// array.
export function useEntityDependencies(
  kind: EntityKind,
  id: string | undefined
) {
  const path =
    kind === 'product'
      ? `/api/products/${id}/dependencies`
      : kind === 'task'
        ? `/api/tasks/${id}/dependencies?direction=out`
        : `/api/manifests/${id}/dependencies?direction=out`
  return useQuery({
    queryKey: entityKeys.deps(kind, id ?? ''),
    queryFn: async () => {
      const res = await fetch(path)
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

export function useEntityComments(
  kind: EntityKind,
  id: string | undefined
) {
  return useQuery({
    queryKey: entityKeys.comments(kind, id ?? ''),
    queryFn: async () => {
      const res = await fetch(`${basePath(kind)}/${id}/comments`)
      if (!res.ok) throw new Error(`comments → ${res.status}`)
      const data = (await res.json()) as Comment[] | { comments: Comment[] }
      return Array.isArray(data) ? data : (data.comments ?? [])
    },
    enabled: !!id,
    staleTime: 15 * 1000,
  })
}

export interface DescriptionRevision {
  id: string
  version: number
  author: string
  body: string
  created_at: number | string
  created_at_iso?: string
}

export function useEntityDescriptionHistory(
  kind: EntityKind,
  id: string | undefined
) {
  return useQuery({
    queryKey: entityKeys.descriptionHistory(kind, id ?? ''),
    queryFn: async () => {
      const res = await fetch(`${basePath(kind)}/${id}/description/history`)
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

// ── Mutations ─────────────────────────────────────────────────────────

export function useUpdateEntity(
  kind: EntityKind,
  id: string | undefined
) {
  const qc = useQueryClient()
  // Tasks update via PATCH; products + manifests via PUT. Backend
  // surface predates the generic queries layer, so we keep the verbs
  // as-is rather than churn handler signatures.
  const method = kind === 'task' ? 'PATCH' : 'PUT'
  return useMutation({
    mutationFn: async (patch: Partial<EntityRecord>) => {
      const res = await fetch(`${basePath(kind)}/${id}`, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch),
      })
      if (!res.ok) throw new Error(`update ${kind} → ${res.status}`)
      return (await res.json()) as EntityRecord
    },
    onSuccess: () => {
      if (id) qc.invalidateQueries({ queryKey: entityKeys.detail(kind, id) })
      qc.invalidateQueries({ queryKey: entityKeys.list(kind) })
      if (id)
        qc.invalidateQueries({ queryKey: entityKeys.comments(kind, id) })
      if (id)
        qc.invalidateQueries({
          queryKey: entityKeys.descriptionHistory(kind, id),
        })
    },
  })
}

export function useChangeEntityStatus(
  kind: EntityKind,
  id: string | undefined
) {
  return useUpdateEntity(kind, id)
}

export function useCreateEntityComment(
  kind: EntityKind,
  id: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      author: string
      type: string
      body: string
    }) => {
      const res = await fetch(`${basePath(kind)}/${id}/comments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw new Error(`post comment → ${res.status}`)
      return (await res.json()) as Comment
    },
    onSuccess: () => {
      if (id)
        qc.invalidateQueries({ queryKey: entityKeys.comments(kind, id) })
    },
  })
}

// ── Dependency mutations ─────────────────────────────────────────────
//
// Same body-style POST + path-style DELETE on both /api/products and
// /api/manifests endpoints. Edges have direction "X depends on Y".
//
//   Upstream  — THIS depends on X.   POST {this} body {depends_on_id: X}
//   Downstream — X depends on THIS.  POST {X}    body {depends_on_id: this}
//   Owned child (product → manifest only) — re-parent via manifest PUT.

interface DepRevisionPayload {
  op: 'add' | 'remove' | 'restore'
  kind:
    | 'product_upstream'
    | 'product_downstream'
    | 'manifest'
    | 'manifest_upstream'
    | 'manifest_downstream'
    | 'snapshot'
  target: { id: string; marker: string; title: string }
  snapshot: {
    upstream: string[]
    downstream: string[]
    manifests: string[]
  }
}

async function postDepRevision(
  kind: EntityKind,
  entityId: string,
  payload: DepRevisionPayload
): Promise<void> {
  const body = JSON.stringify(payload, null, 2)
  await fetch(`${basePath(kind)}/${entityId}/comments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      author: 'operator',
      type: 'agent_note',
      body: '<dependency_revision>\n' + body + '\n</dependency_revision>',
    }),
  })
}

function invalidateDepCaches(
  qc: ReturnType<typeof useQueryClient>,
  kind: EntityKind,
  id: string
) {
  qc.invalidateQueries({ queryKey: entityKeys.deps(kind, id) })
  qc.invalidateQueries({ queryKey: entityKeys.children(kind, id) })
  qc.invalidateQueries({ queryKey: entityKeys.hierarchy(kind, id) })
  qc.invalidateQueries({ queryKey: entityKeys.comments(kind, id) })
}

// 1. Upstream — THIS depends on X.
export function useAddUpstreamDep(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(`${basePath(kind)}/${entityId}/dependencies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ depends_on_id: input.target.id }),
      })
      if (!res.ok && res.status !== 409)
        throw new Error(`add upstream → ${res.status}`)
      if (entityId)
        await postDepRevision(kind, entityId, {
          op: 'add',
          kind:
            kind === 'product' ? 'product_upstream' : 'manifest_upstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      entityId && invalidateDepCaches(qc, kind, entityId),
  })
}

export function useRemoveUpstreamDep(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `${basePath(kind)}/${entityId}/dependencies/${input.target.id}`,
        { method: 'DELETE' }
      )
      if (!res.ok && res.status !== 404)
        throw new Error(`remove upstream → ${res.status}`)
      if (entityId)
        await postDepRevision(kind, entityId, {
          op: 'remove',
          kind:
            kind === 'product' ? 'product_upstream' : 'manifest_upstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      entityId && invalidateDepCaches(qc, kind, entityId),
  })
}

// 2. Downstream — X depends on THIS. Edits go against X's deps API.
export function useAddDownstreamDep(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `${basePath(kind)}/${input.target.id}/dependencies`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: entityId }),
        }
      )
      if (!res.ok && res.status !== 409)
        throw new Error(`add downstream → ${res.status}`)
      if (entityId)
        await postDepRevision(kind, entityId, {
          op: 'add',
          kind:
            kind === 'product' ? 'product_downstream' : 'manifest_downstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      entityId && invalidateDepCaches(qc, kind, entityId),
  })
}

export function useRemoveDownstreamDep(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: { id: string; marker: string; title: string }
      snapshot: DepRevisionPayload['snapshot']
    }) => {
      const res = await fetch(
        `${basePath(kind)}/${input.target.id}/dependencies/${entityId}`,
        { method: 'DELETE' }
      )
      if (!res.ok && res.status !== 404)
        throw new Error(`remove downstream → ${res.status}`)
      if (entityId)
        await postDepRevision(kind, entityId, {
          op: 'remove',
          kind:
            kind === 'product' ? 'product_downstream' : 'manifest_downstream',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      entityId && invalidateDepCaches(qc, kind, entityId),
  })
}

// 3. Manifest re-parent — products only. (Manifests don't own
// manifests; that direction is meaningless.)
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
        await postDepRevision('product', productId, {
          op: 'add',
          kind: 'manifest',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      productId && invalidateDepCaches(qc, 'product', productId),
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
        await postDepRevision('product', productId, {
          op: 'remove',
          kind: 'manifest',
          target: input.target,
          snapshot: input.snapshot,
        })
    },
    onSuccess: () =>
      productId && invalidateDepCaches(qc, 'product', productId),
  })
}

// All-records helpers — feed the dependency picker filtered to
// "candidates not already linked." Cached separately from the kind-
// scoped lists because the picker reads them independent of the
// current entity.
export function useAllProducts() {
  return useQuery({
    queryKey: ['products', 'list'],
    queryFn: async () => {
      const res = await fetch('/api/products')
      if (!res.ok) throw new Error(`products → ${res.status}`)
      return (await res.json()) as Product[]
    },
    staleTime: 30 * 1000,
  })
}

export function useAllManifests() {
  return useQuery({
    queryKey: ['manifest', 'list'],
    queryFn: async () => {
      const res = await fetch('/api/manifests')
      if (!res.ok) throw new Error(`manifests → ${res.status}`)
      return (await res.json()) as Manifest[]
    },
    staleTime: 30 * 1000,
  })
}

// Restore — apply a prior dependency snapshot. Branches on kind:
// products restore sub-products + manifests; manifests restore
// upstream + downstream deps.
export function useRestoreEntityDependencySnapshot(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      snapshot: { downstream: string[]; manifests: string[] }
      revisionLabel: string
      currentDownstream: { id: string; marker: string; title: string }[]
      currentManifests: { id: string; marker: string; title: string }[]
    }) => {
      if (!entityId) return
      const targetSubs = new Set(input.snapshot.downstream)
      const targetManifests = new Set(input.snapshot.manifests)
      const currentSubs = new Set(input.currentDownstream.map((r) => r.id))
      const currentManifestSet = new Set(
        input.currentManifests.map((r) => r.id)
      )

      const subsToAdd = [...targetSubs].filter(
        (id) => !currentSubs.has(id)
      )
      const subsToRemove = [...currentSubs].filter(
        (id) => !targetSubs.has(id)
      )

      // Sub re-parents (downstream — X depends on this).
      for (const subId of subsToAdd) {
        const r = await fetch(`${basePath(kind)}/${subId}/dependencies`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: entityId }),
        })
        if (!r.ok && r.status !== 409) {
          throw new Error(`restore: add sub ${subId} → ${r.status}`)
        }
      }
      for (const subId of subsToRemove) {
        const r = await fetch(
          `${basePath(kind)}/${subId}/dependencies/${entityId}`,
          { method: 'DELETE' }
        )
        if (!r.ok && r.status !== 404) {
          throw new Error(`restore: remove sub ${subId} → ${r.status}`)
        }
      }

      // Manifest re-parents only meaningful for products.
      let addedManifests = 0
      let removedManifests = 0
      if (kind === 'product') {
        const manifestsToAdd = [...targetManifests].filter(
          (id) => !currentManifestSet.has(id)
        )
        const manifestsToRemove = [...currentManifestSet].filter(
          (id) => !targetManifests.has(id)
        )
        for (const mId of manifestsToAdd) {
          const r = await fetch(`/api/manifests/${mId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ project_id: entityId }),
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
        addedManifests = manifestsToAdd.length
        removedManifests = manifestsToRemove.length
      }

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
      await fetch(`${basePath(kind)}/${entityId}/comments`, {
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
        addedManifests,
        removedManifests,
      }
    },
    onSuccess: () =>
      entityId && invalidateDepCaches(qc, kind, entityId),
  })
}

// ── Create-and-link (DAG designer) ───────────────────────────────────

export function useCreateAndLinkSubProduct(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      title: string
      snapshot: { upstream: string[]; downstream: string[]; manifests: string[] }
    }) => {
      const createRes = await fetch('/api/products', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: input.title, status: 'draft' }),
      })
      if (!createRes.ok)
        throw new Error(`create product → ${createRes.status}`)
      const created = (await createRes.json()) as Product

      const linkRes = await fetch(
        `/api/products/${created.id}/dependencies`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: productId }),
        }
      )
      if (!linkRes.ok && linkRes.status !== 409)
        throw new Error(`link sub → ${linkRes.status}`)

      if (productId) {
        const newSnapshot = {
          ...input.snapshot,
          downstream: [...input.snapshot.downstream, created.id],
        }
        await postDepRevision('product', productId, {
          op: 'add',
          kind: 'product_downstream',
          target: {
            id: created.id,
            marker: created.marker,
            title: created.title,
          },
          snapshot: newSnapshot,
        })
      }

      return created
    },
    onSuccess: () => {
      if (productId) invalidateDepCaches(qc, 'product', productId)
      qc.invalidateQueries({ queryKey: entityKeys.list('product') })
    },
  })
}

export function useCreateAndLinkManifest(productId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      title: string
      snapshot: { upstream: string[]; downstream: string[]; manifests: string[] }
    }) => {
      const res = await fetch('/api/manifests', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: input.title,
          project_id: productId,
          status: 'draft',
        }),
      })
      if (!res.ok) throw new Error(`create manifest → ${res.status}`)
      const created = (await res.json()) as Manifest

      if (productId) {
        const newSnapshot = {
          ...input.snapshot,
          manifests: [...input.snapshot.manifests, created.id],
        }
        await postDepRevision('product', productId, {
          op: 'add',
          kind: 'manifest',
          target: {
            id: created.id,
            marker: created.marker,
            title: created.title,
          },
          snapshot: newSnapshot,
        })
      }

      return created
    },
    onSuccess: () => {
      if (productId) invalidateDepCaches(qc, 'product', productId)
    },
  })
}

// Manifests' DAG designer create-new flow — create a new manifest that
// depends on THIS manifest. Project_id is inherited from the current
// manifest so the new one stays under the same product.
export function useCreateAndLinkUpstreamManifest(
  manifestId: string | undefined,
  projectId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      title: string
      snapshot: { upstream: string[]; downstream: string[]; manifests: string[] }
    }) => {
      const res = await fetch('/api/manifests', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: input.title,
          project_id: projectId ?? '',
          status: 'draft',
        }),
      })
      if (!res.ok) throw new Error(`create manifest → ${res.status}`)
      const created = (await res.json()) as Manifest

      // THIS depends on created (created is upstream of THIS).
      const linkRes = await fetch(
        `/api/manifests/${manifestId}/dependencies`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: created.id }),
        }
      )
      if (!linkRes.ok && linkRes.status !== 409)
        throw new Error(`link upstream manifest → ${linkRes.status}`)

      if (manifestId) {
        const newSnapshot = {
          ...input.snapshot,
          upstream: [...input.snapshot.upstream, created.id],
        }
        await postDepRevision('manifest', manifestId, {
          op: 'add',
          kind: 'manifest_upstream',
          target: {
            id: created.id,
            marker: created.marker,
            title: created.title,
          },
          snapshot: newSnapshot,
        })
      }

      return created
    },
    onSuccess: () => {
      if (manifestId) invalidateDepCaches(qc, 'manifest', manifestId)
      qc.invalidateQueries({ queryKey: entityKeys.list('manifest') })
    },
  })
}

// ── Task live output ─────────────────────────────────────────────────
//
// `/api/tasks/{id}/output` returns `{lines: string[], running: bool}`
// from the runner's 200-line ring buffer. Polled while running, then
// settles. When the task is no longer running, the last completed run's
// full `output` blob is the source of truth — fetched via the runs hook.

export interface TaskOutputResponse {
  lines: string[]
  running: boolean
}

export function useTaskOutput(taskId: string | undefined) {
  return useQuery({
    queryKey: ['task', taskId ?? '', 'output'],
    queryFn: () => fetchJSON<TaskOutputResponse>(`/api/tasks/${taskId}/output`),
    enabled: !!taskId,
    // Poll while running; once `running:false` the query stops refetching
    // automatically (refetchInterval reads the latest data).
    refetchInterval: (q) => (q.state.data?.running ? 750 : false),
    refetchIntervalInBackground: false,
    staleTime: 0,
  })
}

export interface TaskRunRow {
  id: number
  task_id: string
  run_number: number
  output: string
  status: string
  actions: number
  lines: number
  cost_usd: number
  turns: number
  model: string
  started_at: string
  completed_at: string
}

// Most-recent run for this task — used to source the full output blob
// once the runner ring buffer has been flushed (i.e. task no longer
// running). The list endpoint returns runs ordered DESC by run_number.
export function useTaskLatestRun(taskId: string | undefined) {
  return useQuery({
    queryKey: ['task', taskId ?? '', 'latest-run'],
    queryFn: async () => {
      const rows = await fetchJSON<TaskRunRow[]>(`/api/tasks/${taskId}/runs`)
      return rows[0] ?? null
    },
    enabled: !!taskId,
    staleTime: 5 * 1000,
  })
}
