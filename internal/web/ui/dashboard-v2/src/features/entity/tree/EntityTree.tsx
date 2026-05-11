import { useCallback, useEffect, useRef, useState, useDeferredValue } from 'react'
import { Search } from 'lucide-react'
import { Tree } from 'react-arborist'
import type { NodeApi } from 'react-arborist'
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
const GROUP_ENTITIES = '__entities__'
const GROUP_SKILLS = '__skills__'
const GROUP_RAG = '__rag__'
const GROUP_MANIFESTS = '__manifests__'
const GROUP_TASKS = '__tasks__'
const GROUP_PRODUCTS = '__products__'
const GROUP_IDEAS = '__ideas__'
const GROUP_PAGES = '__pages__'
const GROUP_IDS: ReadonlySet<string> = new Set([GROUP_ENTITIES, GROUP_SKILLS, GROUP_RAG, GROUP_MANIFESTS, GROUP_TASKS, GROUP_PRODUCTS, GROUP_IDEAS, GROUP_PAGES])

// Page nav nodes injected into the tree so ALL navigation goes through arborist.
const PAGE_URLS: Record<string, string> = {
  __page_overview__: '/',
  __page_actions__: '/actions',
  __page_schedules__: '/schedules',
  __page_inbox__: '/inbox',
  __page_recall__: '/recall',
  __page_stats__: '/stats',
  __page_productivity__: '/productivity',
  __page_audit__: '/audit',
  __page_activity__: '/activity',
  __page_settings__: '/settings',
}

const PAGE_NODES = Object.entries(PAGE_URLS).map(([id, _]) => ({
  id,
  name: {
    __page_overview__: 'Overview',
    __page_actions__: 'Actions Log',
    __page_schedules__: 'Schedules',
    __page_inbox__: 'Inbox',
    __page_recall__: 'Recall',
    __page_stats__: 'Stats',
    __page_productivity__: 'Productivity',
    __page_audit__: 'Audit',
    __page_activity__: 'Activity',
    __page_settings__: 'Settings',
  }[id] ?? id,
  kind: 'page',
  status: '',
}))

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
  const [filterInput, setFilterInput] = useState('')
  const filter = useDeferredValue(filterInput)

  const liveIds = new Set(
    (liveRuns ?? [])
      .filter((r) => r.entity_uid && r.entity_uid !== STDIO_SENTINEL)
      .map((r) => r.entity_uid),
  )
  const overlaid = rawTree ? overlayLiveStatus(rawTree, liveIds) : null

  const products = overlaid?.lifecycle.filter((n) => n.kind === 'product') ?? []
  const ideas = overlaid?.lifecycle.filter((n) => n.kind === 'idea') ?? []

  const treeData: TreeNode[] = [
    {
      id: GROUP_ENTITIES,
      name: 'Entities',
      kind: '__group__',
      status: '',
      children: overlaid
        ? [
            { id: GROUP_SKILLS,    name: 'Skills',    kind: '__group__', status: '', children: overlaid.skills },
            { id: GROUP_RAG,       name: 'RAG',       kind: '__group__', status: '', children: overlaid.rags },
            { id: GROUP_IDEAS,     name: 'Ideas',     kind: '__group__', status: '', children: ideas },
            { id: GROUP_PRODUCTS,  name: 'Products',  kind: '__group__', status: '', children: products },
            { id: GROUP_MANIFESTS, name: 'Manifests', kind: '__group__', status: '', children: overlaid.manifests },
            { id: GROUP_TASKS,     name: 'Tasks',     kind: '__group__', status: '', children: overlaid.tasks },
          ]
        : [],
    },
    {
      id: GROUP_PAGES,
      name: 'Navigation',
      kind: '__group__',
      status: '',
      children: PAGE_NODES,
    },
  ]

  const onSelect = useCallback(
    (nodes: NodeApi<TreeNode>[]) => {
      const node = nodes[0]
      if (!node || GROUP_IDS.has(node.id)) return
      const url = PAGE_URLS[node.id]
      if (url) {
        navigate({ to: url })
      } else {
        navigate({
          to: '/entities/$uid',
          params: { uid: node.id },
          search: { kind: node.data.kind, tab: 'main' },
        })
      }
    },
    [navigate],
  )

  return (
    <div className='flex flex-col flex-1 min-h-0 w-full'>
      {/* Filter input — VS Code style search in tree */}
      <div className='relative px-2 py-1 shrink-0'>
        <Search className='absolute left-4 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground' />
        <input
          value={filterInput}
          onChange={e => setFilterInput(e.target.value)}
          placeholder='Filter...'
          className='w-full pl-6 pr-2 py-1 text-xs bg-white/5 rounded border-0 outline-none text-foreground placeholder:text-muted-foreground focus:bg-white/10'
        />
      </div>
      {/* Tree — flex-1 fills remaining sidebar height */}
      <div ref={ref} className='flex-1 min-h-0 w-full overflow-hidden'>
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
            initialOpenState={{ [GROUP_ENTITIES]: true, [GROUP_PAGES]: true }}
            searchTerm={filter}
            searchMatch={(node, term) => {
              const t = term.toLowerCase()
              if (node.data.name.toLowerCase().includes(t)) return true
              // If any ancestor matches, show this node too so the full
              // subtree is visible when searching by parent name.
              let p = node.parent
              while (p) {
                if (p.data?.name?.toLowerCase().includes(t)) return true
                p = p.parent
              }
              return false
            }}
            className='scrollbar-thin'
          >
            {EntityTreeNode}
          </Tree>
        ) : null}
      </div>
    </div>
  )
}
