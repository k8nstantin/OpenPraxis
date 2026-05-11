import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  Comment,
  Entity,
  ExecutionRow,
  HierarchyNode,
  OutputChunk,
  ProductDependency,
} from '@/lib/types'

// Generic entity queries layer for Products + Manifests + Tasks.
//
// All three kinds share the same 5-tab surface; the only thing that
// changes is the type param. Each hook takes a `kind` and dispatches
// to the unified /api/entities endpoint. See features/entity/* for the
// React surface that consumes these.
//
// Path conventions (unified entities API):
//   GET    /api/entities?type=<kind>&status=<s>&limit=<n>   list
//   GET    /api/entities/:id                                 detail
//   PUT    /api/entities/:id                                 update
//   POST   /api/entities                                     create
//   GET    /api/entities/:id/history                         SCD-2 version history
//   GET    /api/entities/:id/runs                            execution history (ExecutionRow[])
//   GET    /api/entities/:id/comments                        Comments tab
//   GET    /api/entities/search?q=...&type=...               search
//   GET    /api/execution/:runUid                            all events for a run
//   GET    /api/execution/:runUid/output                     live text chunks (OutputChunk[])
//
// Legacy per-entity-type endpoints still in use (backend keeps them registered):
//   /api/relationships/graph                                 DAG tab
//   /api/products/{id}/dependencies                          Dep mutations (product downstream)
//   /api/manifests/{id}/dependencies                         Dep mutations (manifest upstream)
//   /api/products/{id}/hierarchy                             Product list-pane tree expand
//   /api/{kind}s/{id}/settings                               Execution/Main tabs knobs
//   /api/{kind}s/{id}/description/history                    Main tab revisions

// KIND is the single source of truth for entity-kind string values.
// EntityKind below is derived from these so the type and the runtime
// constants cannot drift. Add a new entity kind here and the type
// (plus everything keyed by it) updates in one place.
export const KIND = {
  product: 'product',
  manifest: 'manifest',
  task: 'task',
  skill: 'skill',
  idea: 'idea',
  RAG: 'RAG',
} as const

export type EntityKind = (typeof KIND)[keyof typeof KIND]

// AnyEntityKind accepts the 6 known built-in kinds (with full autocomplete)
// plus any arbitrary string for DB-driven dynamic types. Use this wherever
// custom/user-defined kinds may flow through instead of EntityKind.
// eslint-disable-next-line @typescript-eslint/ban-types
export type AnyEntityKind = EntityKind | (string & {})

// EntityRecord is the unified entity shape returned by /api/entities.
export type EntityRecord = Entity

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
  runs: (kind: EntityKind, id: string) =>
    [...entityKeys.all(kind), 'runs', id] as const,
  execution: (runUid: string) => ['execution', runUid] as const,
  executionOutput: (runUid: string) => ['execution', runUid, 'output'] as const,
}

// kindPlural returns the legacy URL segment for a given entity kind.
// Used only for endpoints that are still registered per-kind in the
// backend: /api/{kind}s/{id}/settings and /api/{kind}s/{id}/dependencies.
function kindPlural(kind: EntityKind): string {
  if (kind === 'product') return 'products'
  if (kind === 'task') return 'tasks'
  return 'manifests'
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
  kind: AnyEntityKind
  title: string
  status: string
}
export interface GraphEdge {
  id: string
  source: string
  target: string
  kind: 'owns' | 'depends_on'
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
    queryFn: () =>
      fetchJSON<Entity[]>(`/api/entities?type=${kind}&limit=200`),
    staleTime: 30 * 1000,
  })
}

export function useEntity(kind: EntityKind, id: string | undefined) {
  return useQuery({
    queryKey: entityKeys.detail(kind, id ?? ''),
    queryFn: () => fetchJSON<Entity>(`/api/entities/${id}`),
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
    // Only the /api/products/{id}/hierarchy endpoint exists in the backend.
    queryFn: () =>
      fetchJSON<HierarchyNode>(`/api/products/${id}/hierarchy`),
    enabled: kind === 'product' && !!id,
    staleTime: 30 * 1000,
  })
}

// Children — products → manifests; manifests → tasks; tasks → none.
// Same shape on the wire ({entity_uid, title, status} subset), so the
// consumers (Dependencies tab + DAG tab) can treat the rows uniformly.
// Tasks are leaves: the query is disabled and resolves to an empty
// array so callers don't have to special-case undefined.
export function useEntityChildren(
  kind: EntityKind,
  id: string | undefined
) {
  return useQuery({
    queryKey: entityKeys.children(kind, id ?? ''),
    queryFn: async () => {
      if (!id || kind === 'task') return []
      // Use relationships graph — direct owns children only (depth=1)
      const childKind = kind === 'product' ? 'manifest' : 'task'
      const res = await fetch(
        `/api/relationships/graph?root_id=${id}&root_kind=${kind}&depth=1`
      )
      if (!res.ok) return []
      const data = (await res.json()) as { nodes: { id: string; kind: string; title: string; status: string }[] }
      // Filter to direct children of the correct type (exclude the root itself)
      return (data.nodes ?? []).filter(n => n.id !== id && n.kind === childKind)
    },
    enabled: !!id && kind !== 'task',
    staleTime: 15 * 1000,
  })
}

// Dependency rows — same {id, title, status} shape. Uses the per-kind
// dependency endpoints (products and manifests only; tasks are leaves).
// Backend registers: /api/products/{id}/dependencies and
// /api/manifests/{id}/dependencies.
export function useEntityDependencies(
  kind: EntityKind,
  id: string | undefined
) {
  // Tasks have no dep endpoint in the backend — return empty for them.
  const path =
    kind === 'product'
      ? `/api/products/${id}/dependencies`
      : kind === 'manifest'
        ? `/api/manifests/${id}/dependencies?direction=out`
        : ''
  return useQuery({
    queryKey: entityKeys.deps(kind, id ?? ''),
    queryFn: async () => {
      if (!path) return [] as ProductDependency[]
      const res = await fetch(path)
      if (!res.ok) throw new Error(`dependencies → ${res.status}`)
      const data = (await res.json()) as
        | ProductDependency[]
        | { deps: ProductDependency[] }
      return Array.isArray(data) ? data : (data.deps ?? [])
    },
    enabled: !!id && kind !== 'task',
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
      // Unified /api/entities/:id/comments endpoint — works for all kinds.
      const res = await fetch(`/api/entities/${id}/comments`)
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
      // Description revisions are stored as comments with type=prompt.
      const res = await fetch(`/api/entities/${id}/comments?type=prompt&limit=100`)
      if (!res.ok) throw new Error(`description history → ${res.status}`)
      const data = (await res.json()) as { comments: DescriptionRevision[] } | DescriptionRevision[]
      const rows = Array.isArray(data) ? data : (data.comments ?? [])
      // Map comment shape {author, body, created_at} to DescriptionRevision shape.
      return rows.map((r, i) => ({
        id: (r as {id?: string}).id ?? String(i),
        version: rows.length - i,
        author: (r as {author?: string}).author ?? '',
        body: (r as {body?: string}).body ?? '',
        created_at: (r as {created_at?: number | string}).created_at ?? '',
      })) as DescriptionRevision[]
    },
    enabled: !!id,
    staleTime: 60 * 1000,
  })
}

// Entity execution runs — new unified endpoint.
export function useEntityRuns(
  kind: EntityKind,
  id: string | undefined
) {
  return useQuery({
    queryKey: entityKeys.runs(kind, id ?? ''),
    queryFn: () =>
      fetchJSON<ExecutionRow[]>(`/api/entities/${id}/runs`),
    enabled: !!id,
    staleTime: 10 * 1000,
  })
}

// All events for a specific run (started|sample|completed|failed rows).
export function useExecutionRun(runUid: string | undefined) {
  return useQuery({
    queryKey: entityKeys.execution(runUid ?? ''),
    queryFn: () =>
      fetchJSON<ExecutionRow[]>(`/api/execution/${runUid}`),
    enabled: !!runUid,
    staleTime: 10 * 1000,
  })
}

// Live text chunks for a run's output stream.
// Pass isLive=false for completed runs to disable polling entirely.
export function useExecutionOutput(runUid: string | undefined, isLive = true) {
  return useQuery({
    queryKey: entityKeys.executionOutput(runUid ?? ''),
    queryFn: () =>
      fetchJSON<OutputChunk[]>(`/api/execution/${runUid}/output`),
    enabled: !!runUid,
    refetchInterval: isLive ? 750 : false,
    refetchIntervalInBackground: false,
    staleTime: isLive ? 0 : 60_000,
  })
}

// ── Mutations ─────────────────────────────────────────────────────────

// Create a new entity of any type. Returns the created entity so the
// caller can immediately navigate to it. Accepts AnyEntityKind so
// DB-driven dynamic types are not rejected at the call site.
export function useCreateEntity(kind: AnyEntityKind) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (payload: { type: AnyEntityKind; title: string; status?: string; tags?: string[] }) => {
      const res = await fetch('/api/entities', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...payload, status: payload.status ?? 'draft' }),
      })
      if (!res.ok) throw new Error(`create ${kind} → ${res.status}`)
      return (await res.json()) as Entity
    },
    onSuccess: () => {
      // Invalidate the list for this kind if it is a known EntityKind.
      // For dynamic kinds the query key still matches the string value so
      // callers that used entityKeys.list() for the same kind will refresh.
      qc.invalidateQueries({ queryKey: [kind, 'list'] })
    },
  })
}

export function useUpdateEntity(
  kind: EntityKind,
  id: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (patch: Partial<EntityRecord>) => {
      const res = await fetch(`/api/entities/${id}`, {
        method: 'PUT',
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
      // Use the unified /api/entities/:id/comments endpoint.
      const res = await fetch(`/api/entities/${id}/comments`, {
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
  await fetch(`/api/entities/${entityId}/comments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      author: 'operator',
      type: 'comment',
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
      const res = await fetch(`/api/${kindPlural(kind)}/${entityId}/dependencies`, {
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
        `/api/${kindPlural(kind)}/${entityId}/dependencies/${input.target.id}`,
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
        `/api/${kindPlural(kind)}/${input.target.id}/dependencies`,
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
        `/api/${kindPlural(kind)}/${input.target.id}/dependencies/${entityId}`,
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
      const res = await fetch(`/api/entities/${input.target.id}`, {
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
      const res = await fetch(`/api/entities/${input.target.id}`, {
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
      const res = await fetch('/api/entities?type=product&limit=500')
      if (!res.ok) throw new Error(`products → ${res.status}`)
      return (await res.json()) as Entity[]
    },
    staleTime: 30 * 1000,
  })
}

export function useAllManifests() {
  return useQuery({
    queryKey: ['manifest', 'list'],
    queryFn: async () => {
      const res = await fetch('/api/entities?type=manifest&limit=500')
      if (!res.ok) throw new Error(`manifests → ${res.status}`)
      return (await res.json()) as Entity[]
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
        const r = await fetch(`/api/${kindPlural(kind)}/${subId}/dependencies`, {
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
          `/api/${kindPlural(kind)}/${subId}/dependencies/${entityId}`,
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
          const r = await fetch(`/api/entities/${mId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ project_id: entityId }),
          })
          if (!r.ok)
            throw new Error(`restore: link manifest ${mId} → ${r.status}`)
        }
        for (const mId of manifestsToRemove) {
          const r = await fetch(`/api/entities/${mId}`, {
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
      await fetch(`/api/entities/${entityId}/comments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          author: 'operator',
          type: 'comment',
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
      const createRes = await fetch('/api/entities', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: input.title, type: 'product', status: 'draft' }),
      })
      if (!createRes.ok)
        throw new Error(`create product → ${createRes.status}`)
      const created = (await createRes.json()) as Entity

      const createdId = created.entity_uid
      const linkRes = await fetch(
        `/api/products/${createdId}/dependencies`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: productId }),
        }
      )
      // 409 = already linked — treat as success.
      if (!linkRes.ok && linkRes.status !== 409)
        throw new Error(`link sub → ${linkRes.status}`)

      if (productId) {
        const newSnapshot = {
          ...input.snapshot,
          downstream: [...input.snapshot.downstream, createdId],
        }
        await postDepRevision('product', productId, {
          op: 'add',
          kind: 'product_downstream',
          target: {
            id: createdId,
            marker: createdId.slice(0, 12),
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
      const res = await fetch('/api/entities', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: input.title,
          type: 'manifest',
          project_id: productId,
          status: 'draft',
        }),
      })
      if (!res.ok) throw new Error(`create manifest → ${res.status}`)
      const created = (await res.json()) as Entity
      const createdId = created.entity_uid

      if (productId) {
        const newSnapshot = {
          ...input.snapshot,
          manifests: [...input.snapshot.manifests, createdId],
        }
        await postDepRevision('product', productId, {
          op: 'add',
          kind: 'manifest',
          target: {
            id: createdId,
            marker: createdId.slice(0, 12),
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
      const res = await fetch('/api/entities', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: input.title,
          type: 'manifest',
          project_id: projectId ?? '',
          status: 'draft',
        }),
      })
      if (!res.ok) throw new Error(`create manifest → ${res.status}`)
      const created = (await res.json()) as Entity
      const createdId = created.entity_uid

      // THIS depends on created (created is upstream of THIS).
      const linkRes = await fetch(
        `/api/manifests/${manifestId}/dependencies`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ depends_on_id: createdId }),
        }
      )
      // 409 = already linked — treat as success.
      if (!linkRes.ok && linkRes.status !== 409)
        throw new Error(`link upstream manifest → ${linkRes.status}`)

      if (manifestId) {
        const newSnapshot = {
          ...input.snapshot,
          upstream: [...input.snapshot.upstream, createdId],
        }
        await postDepRevision('manifest', manifestId, {
          op: 'add',
          kind: 'manifest_upstream',
          target: {
            id: createdId,
            marker: createdId.slice(0, 12),
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
// The legacy `/api/tasks/{id}/output` ring-buffer endpoint was never
// implemented in the backend. Live output is now served via:
//
//   GET /api/entities/{id}/runs   → ExecutionRow[] (all run events)
//   GET /api/execution/{runUid}/output → OutputChunk[] (text chunks)
//
// `useTaskRuns` fetches the full run event log so the Live Output tab
// can group events by run_uid and determine running/completed state.
// `useTaskRunOutput` fetches the text chunks for a specific run.

// All run events for this task from /api/entities/{id}/runs.
// Returns ExecutionRow[] sorted DESC by created_at (backend order).
export function useTaskRuns(taskId: string | undefined) {
  return useQuery({
    queryKey: ['task', taskId ?? '', 'runs'],
    queryFn: () => fetchJSON<ExecutionRow[]>(`/api/entities/${taskId}/runs`),
    enabled: !!taskId,
    refetchInterval: (q) => {
      // Keep polling while there is an active run (started without a terminal event).
      const rows = q.state.data ?? []
      const runUids = new Set(rows.map((r) => r.run_uid))
      const started = new Set(
        rows.filter((r) => r.event === 'started').map((r) => r.run_uid)
      )
      const terminal = new Set(
        rows
          .filter((r) => r.event === 'completed' || r.event === 'failed')
          .map((r) => r.run_uid)
      )
      const hasRunning = [...runUids].some(
        (uid) => started.has(uid) && !terminal.has(uid)
      )
      return hasRunning ? 1500 : false
    },
    refetchIntervalInBackground: false,
    staleTime: 5 * 1000,
  })
}

// Text chunks for a specific run — used by the Live Output tab for the
// current in-flight run. Delegates to useExecutionOutput (same hook).
export { useExecutionOutput as useTaskRunOutput }

export interface LiveRun {
  run_uid: string
  entity_uid: string
  entity_title: string
  entity_type: string
  elapsed_sec: number
  turns: number
  actions: number
  cost_usd: number
  model: string
}

// Polls /api/execution/live — shared query used by list-pane to highlight
// running entities and by RunsTab for the live run header.
export function useLiveRuns() {
  return useQuery({
    queryKey: ['execution-live'],
    queryFn: (): Promise<LiveRun[]> => fetchJSON<LiveRun[]>('/api/execution/live'),
    refetchInterval: (q) => ((q.state.data?.length ?? 0) > 0 ? 4000 : 10000),
    refetchIntervalInBackground: false,
  })
}
