import { useCallback, useEffect, useRef, useState } from 'react'
import { Tree } from 'react-arborist'
import { useNavigate } from '@tanstack/react-router'
import { useLiveRuns } from '@/lib/queries/entity'
import {
  TreeStatus,
  overlayLiveStatus,
  useEntityTree,
  type TreeNode,
} from '@/lib/queries/entity-tree'
import { Skeleton } from '@/components/ui/skeleton'
import { EntityTreeNode } from './EntityTreeNode'

// Synthetic root group ids — surfaced as sticky headers, not navigable.
const GROUP_SKILLS = '__skills__'
const GROUP_LIFECYCLE = '__lifecycle__'
const GROUP_IDS: ReadonlySet<string> = new Set([GROUP_SKILLS, GROUP_LIFECYCLE])

// Live-runs sentinel for stdio sessions (no entity_uid). Filtered out so the
// tree doesn't try to mark a non-existent node as running.
const STDIO_SENTINEL = 'stdio'

// react-arborist requires exact pixel width/height — CSS percentages cause
// virtualized rows (rendered at absolute positions) to overflow horizontally.
// useElementSize feeds live ResizeObserver readouts so the Tree mounts only
// once the container has real dimensions. Inlined to avoid the
// use-resize-observer peer-dep range mismatch with React 19.
function useElementSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  const [size, setSize] = useState<{ width: number; height: number }>({
    width: 0,
    height: 0,
  })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    // Seed from initial layout so we don't wait for the first ResizeObserver
    // tick (which may not fire on mount in StrictMode dev double-invoke).
    setSize({ width: el.clientWidth, height: el.clientHeight })
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect
      if (cr) setSize({ width: cr.width, height: cr.height })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return { ref, ...size }
}

export function EntityTree() {
  const { ref, width, height } = useElementSize<HTMLDivElement>()
  const { data: rawTree, isLoading } = useEntityTree()
  const { data: liveRuns } = useLiveRuns()
  const navigate = useNavigate()

  const liveIds = new Set(
    (liveRuns ?? [])
      .filter((r) => r.entity_uid && r.entity_uid !== STDIO_SENTINEL)
      .map((r) => r.entity_uid),
  )
  const overlaid = rawTree ? overlayLiveStatus(rawTree, liveIds) : null

  const treeData: TreeNode[] = overlaid
    ? [
        {
          id: GROUP_SKILLS,
          name: 'Skills',
          kind: 'skill',
          status: TreeStatus.Active,
          children: overlaid.skills,
        },
        {
          id: GROUP_LIFECYCLE,
          name: 'Entities',
          kind: 'idea',
          status: TreeStatus.Active,
          children: overlaid.lifecycle,
        },
      ]
    : []

  const onSelect = useCallback(
    (nodes: { id: string }[]) => {
      const node = nodes[0]
      if (!node || GROUP_IDS.has(node.id)) return
      navigate({ to: '/entities/$uid', params: { uid: node.id } })
    },
    [navigate],
  )

  // Single mount point: ref is always attached so the ResizeObserver can fire
  // before the query settles. Skeleton, empty state, and Tree share the box.
  return (
    <div ref={ref} className='h-[420px] w-full overflow-hidden'>
      {isLoading ? (
        <div className='px-3 space-y-1 pt-2'>
          {[1, 2, 3, 4, 5].map((i) => (
            <Skeleton key={i} className='h-5 w-full' />
          ))}
        </div>
      ) : width > 0 && height > 0 ? (
        <Tree
          data={treeData}
          width={width}
          height={height}
          indent={16}
          rowHeight={24}
          overscanCount={8}
          onSelect={onSelect}
          openByDefault={false}
          initialOpenState={{ [GROUP_SKILLS]: true, [GROUP_LIFECYCLE]: true }}
          className='scrollbar-thin'
        >
          {EntityTreeNode}
        </Tree>
      ) : null}
    </div>
  )
}
