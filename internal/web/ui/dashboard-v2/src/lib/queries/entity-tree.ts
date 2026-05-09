import { useQuery } from '@tanstack/react-query'

import type { EntityKind } from './entity'

// useEntityTree — assembles the full sidebar tree from REST endpoints.
//
// Two top-level groups feed react-arborist:
//   1. Skills        (global governance — flat leaf list)
//   2. Lifecycle     (Idea → Product → Manifest → Task, nested via DAG edges)
//
// All relationship lookups within a depth fan out via Promise.all — the
// whole tree resolves in (number-of-levels) round-trips, not per-node.
//
// Status on parent nodes is derived client-side from children
// (see deriveStatus). Parent rows in `entities` carry their own status
// (active/draft/closed/archived), but execution liveness lives on tasks
// — we surface that upward so the tree shows aggregate health.

// ── Single-point-of-change constants ─────────────────────────────────
// Domain values that flow into URLs. Defined once here so a rename in
// the backend requires changing exactly one line. Re-exported for any
// consumer that branches on kind/status (renderers, navigation, etc.).
export const KIND = {
  skill: 'skill',
  idea: 'idea',
  product: 'product',
  manifest: 'manifest',
  task: 'task',
} as const satisfies Record<EntityKind, EntityKind>

const EDGE = {
  owns: 'owns',
  linksTo: 'links_to',
} as const

export const STATUS = {
  draft: 'draft',
  active: 'active',
  running: 'running',
  completed: 'completed',
  failed: 'failed',
  closed: 'closed',
  archived: 'archived',
} as const

// ── Types ─────────────────────────────────────────────────────────────

export type TreeStatus = (typeof STATUS)[keyof typeof STATUS]

export interface TreeNode {
  id: string
  name: string
  kind: EntityKind
  status: TreeStatus
  children?: TreeNode[]
}

export interface EntityTree {
  skills: TreeNode[]
  lifecycle: TreeNode[]
}

// ── Wire shapes ───────────────────────────────────────────────────────
// /api/entities returns a flat list with snake_case fields.
interface EntityRow {
  entity_uid: string
  type: EntityKind
  title: string
  status: string
}

// /api/relationships/{incoming,outgoing} returns Go's PascalCase fields.
interface RelationshipRow {
  SrcID: string
  SrcKind: EntityKind
  DstID: string
  DstKind: EntityKind
  Kind: string
}

// ── Fetch helpers ─────────────────────────────────────────────────────

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

async function fetchEntities(
  type: EntityKind,
  params: { status?: string; limit: number },
): Promise<EntityRow[]> {
  const qs = new URLSearchParams({ type, limit: String(params.limit) })
  if (params.status) qs.set('status', params.status)
  const rows = await fetchJSON<EntityRow[] | null>(`/api/entities?${qs}`)
  return rows ?? []
}

async function fetchEntity(id: string): Promise<EntityRow | null> {
  try {
    return await fetchJSON<EntityRow>(`/api/entities/${id}`)
  } catch {
    return null
  }
}

async function fetchOutgoing(
  srcID: string,
  edgeKind: string,
): Promise<RelationshipRow[]> {
  const qs = new URLSearchParams({ src_id: srcID, kind: edgeKind })
  const rows = await fetchJSON<RelationshipRow[] | null>(
    `/api/relationships/outgoing?${qs}`,
  )
  return rows ?? []
}

async function fetchIncoming(
  dstID: string,
  edgeKind: string,
): Promise<RelationshipRow[]> {
  const qs = new URLSearchParams({ dst_id: dstID, kind: edgeKind })
  const rows = await fetchJSON<RelationshipRow[] | null>(
    `/api/relationships/incoming?${qs}`,
  )
  return rows ?? []
}

// ── Status derivation ─────────────────────────────────────────────────

function normalizeStatus(s: string): TreeStatus {
  const known: TreeStatus[] = [
    STATUS.draft,
    STATUS.active,
    STATUS.running,
    STATUS.completed,
    STATUS.failed,
    STATUS.closed,
    STATUS.archived,
  ]
  return (known as string[]).includes(s) ? (s as TreeStatus) : STATUS.draft
}

export function deriveStatus(children: TreeNode[]): TreeStatus {
  if (children.length === 0) return STATUS.draft
  if (children.some((c) => c.status === STATUS.running)) return STATUS.running
  if (children.some((c) => c.status === STATUS.failed)) return STATUS.failed
  if (children.every((c) => c.status === STATUS.completed))
    return STATUS.completed
  if (children.some((c) => c.status === STATUS.completed)) return STATUS.active
  return STATUS.draft
}

// ── Tree builders ─────────────────────────────────────────────────────

async function buildManifestNode(manifest: EntityRow): Promise<TreeNode> {
  const taskEdges = await fetchOutgoing(manifest.entity_uid, EDGE.owns)
  const tasks = await Promise.all(
    taskEdges
      .filter((e) => e.DstKind === KIND.task)
      .map(async (e) => {
        const t = await fetchEntity(e.DstID)
        if (!t) return null
        const node: TreeNode = {
          id: t.entity_uid,
          name: t.title,
          kind: KIND.task,
          status: normalizeStatus(t.status),
        }
        return node
      }),
  )
  const children = tasks.filter((t): t is TreeNode => t !== null)
  return {
    id: manifest.entity_uid,
    name: manifest.title,
    kind: KIND.manifest,
    status: children.length ? deriveStatus(children) : normalizeStatus(manifest.status),
    children,
  }
}

async function buildProductNode(product: EntityRow): Promise<TreeNode> {
  const manifestEdges = await fetchOutgoing(product.entity_uid, EDGE.owns)
  const manifests = await Promise.all(
    manifestEdges
      .filter((e) => e.DstKind === KIND.manifest)
      .map(async (e) => {
        const m = await fetchEntity(e.DstID)
        if (!m) return null
        return buildManifestNode(m)
      }),
  )
  const children = manifests.filter((m): m is TreeNode => m !== null)
  return {
    id: product.entity_uid,
    name: product.title,
    kind: KIND.product,
    status: children.length ? deriveStatus(children) : normalizeStatus(product.status),
    children,
  }
}

async function buildIdeaNode(
  idea: EntityRow,
  linkedProductIDs: Set<string>,
  productByID: Map<string, EntityRow>,
): Promise<TreeNode> {
  const productEdges = await fetchOutgoing(idea.entity_uid, EDGE.linksTo)
  const productIDs = productEdges
    .filter((e) => e.DstKind === KIND.product)
    .map((e) => e.DstID)
  for (const id of productIDs) linkedProductIDs.add(id)

  const products = await Promise.all(
    productIDs.map(async (id) => {
      const p = productByID.get(id) ?? (await fetchEntity(id))
      if (!p) return null
      return buildProductNode(p)
    }),
  )
  const children = products.filter((p): p is TreeNode => p !== null)
  return {
    id: idea.entity_uid,
    name: idea.title,
    kind: KIND.idea,
    status: children.length ? deriveStatus(children) : normalizeStatus(idea.status),
    children: children.length ? children : undefined,
  }
}

async function fetchEntityTree(): Promise<EntityTree> {
  // Group 1 + initial fan-out for Group 2 in parallel.
  const [skillRows, ideaRows, productRows] = await Promise.all([
    fetchEntities(KIND.skill, { status: STATUS.active, limit: 100 }),
    fetchEntities(KIND.idea, { limit: 200 }),
    fetchEntities(KIND.product, { limit: 200 }),
  ])

  const skills: TreeNode[] = skillRows.map((s) => ({
    id: s.entity_uid,
    name: s.title,
    kind: KIND.skill,
    status: normalizeStatus(s.status),
  }))

  const productByID = new Map(productRows.map((p) => [p.entity_uid, p]))
  const linkedProductIDs = new Set<string>()

  const ideaNodes = await Promise.all(
    ideaRows.map((i) => buildIdeaNode(i, linkedProductIDs, productByID)),
  )

  // Products not linked to any idea → "Unlinked Products" group.
  const unlinkedProducts = productRows.filter(
    (p) => !linkedProductIDs.has(p.entity_uid),
  )
  const unlinkedNodes = await Promise.all(
    unlinkedProducts.map((p) => buildProductNode(p)),
  )

  const lifecycle: TreeNode[] = [...ideaNodes]
  if (unlinkedNodes.length) {
    lifecycle.push({
      id: 'unlinked-products',
      name: 'Unlinked Products',
      kind: KIND.idea,
      status: deriveStatus(unlinkedNodes),
      children: unlinkedNodes,
    })
  }

  return { skills, lifecycle }
}

// ── Hook ──────────────────────────────────────────────────────────────

export const entityTreeKeys = {
  all: () => ['entity-tree'] as const,
}

export function useEntityTree() {
  return useQuery({
    queryKey: entityTreeKeys.all(),
    queryFn: fetchEntityTree,
    staleTime: 30_000,
    refetchInterval: 10_000,
  })
}

// Re-exports — let consumers import via this module (tree-friendly).
export { fetchEntityTree }

// Test/dev helpers — exported so children-of-different-shapes tests can
// assert the derivation rules without reaching into the closure.
export const __test__ = { deriveStatus, normalizeStatus }
