import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Tree } from 'react-arborist'
import type { NodeRendererProps } from 'react-arborist'
import type { EntityKind } from '@/lib/queries/entity'
import { useLiveRuns } from '@/lib/queries/entity'
import {
  KIND,
  STATUS,
  entityTreeKeys,
  overlayLiveStatus,
  useEntityTree,
  type TreeNode,
} from '@/lib/queries/entity-tree'
import { EntityTreeNode } from './EntityTreeNode'

// Sentinel `entity_uid` for non-entity runs (bare-stdio MCP sessions).
// Mirrored from runs.tsx — the live-runs endpoint emits this when an
// agent is connected via stdio without a backing task UUID, so it must
// not flip a tree node to running.
const NON_ENTITY_RUN_UID = 'stdio'

// Sentinel IDs for the two synthetic group headers. Prefixed with `__`
// so they cannot collide with real UUID v7 entity IDs.
const GROUP_ID = {
  skills: '__skills__',
  lifecycle: '__lifecycle__',
} as const

const GROUP_ID_SET: ReadonlySet<string> = new Set(Object.values(GROUP_ID))

// Where to navigate when the user selects an entity. The unified
// `/entities/$uid` route is M3/T1's deliverable; until then, drop into
// the kind-scoped page that already exists. Single map => one line to
// flip when M3/T1 lands.
const KIND_TO_PATH: Record<EntityKind, string> = {
  [KIND.product]: '/products',
  [KIND.manifest]: '/manifests',
  [KIND.task]: '/tasks',
  [KIND.skill]: '/skills',
  [KIND.idea]: '/ideas',
}

const OPEN_STATE_KEY = 'entity-tree-open'

function readOpenState(): Record<string, boolean> {
  if (typeof window === 'undefined') return {}
  try {
    const raw = window.localStorage.getItem(OPEN_STATE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') {
      return parsed as Record<string, boolean>
    }
  } catch {
    // corrupted entry — ignore and start fresh
  }
  return {}
}

function writeOpenState(map: Record<string, boolean>): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(OPEN_STATE_KEY, JSON.stringify(map))
  } catch {
    // quota or privacy mode — silently degrade
  }
}

function TreeSkeleton() {
  return (
    <div className='space-y-1.5 px-2 py-1'>
      {Array.from({ length: 6 }).map((_, i) => (
        <div
          key={i}
          className='h-4 bg-sidebar-accent/40 rounded-sm animate-pulse'
          style={{ width: `${60 + ((i * 13) % 35)}%` }}
        />
      ))}
    </div>
  )
}

export function EntityTree() {
  const { data, isLoading } = useEntityTree()
  const liveRuns = useLiveRuns()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  // We can't observe react-arborist's internal open state directly, so
  // mirror it: seeded from localStorage, mutated on every onToggle.
  const openMapRef = useRef<Record<string, boolean>>(readOpenState())
  const [, forceTick] = useState(0)

  // Set of entity ids currently executing — pulled from the live-runs
  // poll (4s while any agent runs, 10s idle). Memoised on a stable
  // signature so referential identity only changes when membership does.
  const liveSignature = useMemo(() => {
    const ids = (liveRuns.data ?? [])
      .map((r) => r.entity_uid)
      .filter((id) => id && id !== NON_ENTITY_RUN_UID)
      .sort()
    return ids.join(',')
  }, [liveRuns.data])

  const liveTaskIds = useMemo<ReadonlySet<string>>(
    () => new Set(liveSignature ? liveSignature.split(',') : []),
    [liveSignature],
  )

  // When the running set flips, invalidate the tree so the next render
  // pulls authoritative status from the API. Until that resolves, the
  // overlay below keeps the dot in sync visually.
  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: entityTreeKeys.all() })
  }, [liveSignature, queryClient])

  const overlaid = useMemo(() => {
    if (!data) return undefined
    return overlayLiveStatus(data, liveTaskIds)
  }, [data, liveTaskIds])

  const treeData = useMemo<TreeNode[]>(
    () => [
      {
        id: GROUP_ID.skills,
        name: 'Skills',
        kind: KIND.skill,
        status: STATUS.active,
        children: overlaid?.skills ?? [],
      },
      {
        id: GROUP_ID.lifecycle,
        name: 'Work Lifecycle',
        kind: KIND.idea,
        status: STATUS.active,
        children: overlaid?.lifecycle ?? [],
      },
    ],
    [overlaid],
  )

  const handleToggle = useCallback((id: string) => {
    const next = { ...openMapRef.current, [id]: !openMapRef.current[id] }
    openMapRef.current = next
    writeOpenState(next)
    forceTick((n) => n + 1)
  }, [])

  const handleSelect = useCallback(
    (nodes: { id: string; data: TreeNode }[]) => {
      const node = nodes[0]
      if (!node || GROUP_ID_SET.has(node.id)) return
      const path = KIND_TO_PATH[node.data.kind]
      if (!path) return
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      navigate({ to: path, search: { id: node.id, tab: 'main' } } as any)
    },
    [navigate],
  )

  if (isLoading) return <TreeSkeleton />

  return (
    <Tree<TreeNode>
      data={treeData}
      openByDefault={false}
      initialOpenState={openMapRef.current}
      onToggle={handleToggle}
      onSelect={handleSelect}
      width='100%'
      indent={16}
      rowHeight={28}
      overscanCount={8}
    >
      {(props: NodeRendererProps<TreeNode>) => {
        if (GROUP_ID_SET.has(props.node.id)) {
          return (
            <div
              style={props.style}
              className='px-2 py-1 text-xs font-semibold text-muted-foreground uppercase tracking-wider'
            >
              {props.node.data.name}
            </div>
          )
        }
        return <EntityTreeNode {...props} />
      }}
    </Tree>
  )
}
