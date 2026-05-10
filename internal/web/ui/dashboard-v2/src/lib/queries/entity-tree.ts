import { useQuery } from '@tanstack/react-query'
import type { EntityKind } from './entity'

// TreeNode mirrors the shape returned by GET /api/entities/tree.
// Status is the wire status string: derived in the backend from entity status
// + child rollup. Kept loose here because the renderer's status maps are the
// canonical home for visualization decisions.
export interface TreeNode {
  id: string
  name: string
  kind: string  // EntityKind for real entities; 'page' for synthetic nav nodes
  status: string
  children?: TreeNode[]
}

export interface EntityTreePayload {
  skills: TreeNode[]
  lifecycle: TreeNode[]
}

async function fetchEntityTree(): Promise<EntityTreePayload> {
  const res = await fetch('/api/entities/tree')
  if (!res.ok) throw new Error(`/api/entities/tree → ${res.status}`)
  const data = (await res.json()) as Partial<EntityTreePayload>
  return { skills: data.skills ?? [], lifecycle: data.lifecycle ?? [] }
}

export function useEntityTree() {
  return useQuery({
    queryKey: ['entity-tree'],
    queryFn: fetchEntityTree,
    staleTime: 30_000,
    refetchInterval: 10_000,
  })
}

// Tree-display status used by the renderer's status visuals. Centralized so
// adding a status visual is a single-file change.
export const TreeStatus = {
  Running: 'running',
  Completed: 'completed',
  Failed: 'failed',
  Active: 'active',
  Draft: 'draft',
  Closed: 'closed',
  Archived: 'archived',
} as const
export type TreeStatus = (typeof TreeStatus)[keyof typeof TreeStatus]

// overlayLiveStatus walks the tree and marks any node whose id is in the live
// set as Running, then re-derives parent status from its children. Pure,
// returns a new tree.
export function overlayLiveStatus(
  tree: EntityTreePayload,
  liveIds: ReadonlySet<string>,
): EntityTreePayload {
  function walk(n: TreeNode): TreeNode {
    const live = liveIds.has(n.id)
    if (n.children?.length) {
      const ch = n.children.map(walk)
      return { ...n, status: live ? TreeStatus.Running : deriveStatus(ch), children: ch }
    }
    return live ? { ...n, status: TreeStatus.Running } : n
  }
  return { skills: tree.skills.map(walk), lifecycle: tree.lifecycle.map(walk) }
}

function deriveStatus(ch: TreeNode[]): string {
  if (ch.some((c) => c.status === TreeStatus.Running)) return TreeStatus.Running
  if (ch.some((c) => c.status === TreeStatus.Failed)) return TreeStatus.Failed
  if (ch.length > 0 && ch.every((c) => c.status === TreeStatus.Completed))
    return TreeStatus.Completed
  if (ch.some((c) => c.status === TreeStatus.Completed)) return TreeStatus.Active
  return TreeStatus.Draft
}
