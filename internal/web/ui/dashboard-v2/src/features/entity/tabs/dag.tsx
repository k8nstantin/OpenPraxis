import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import ELK from 'elkjs/lib/elk.bundled.js'
import { Pencil, Plus, Unlink, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  useAddDownstreamDep,
  useAddUpstreamDep,
  useAllManifests,
  useAllProducts,
  useCreateAndLinkManifest,
  useCreateAndLinkSubProduct,
  useCreateAndLinkUpstreamManifest,
  useEntity,
  useEntityChildren,
  useEntityDependencies,
  useEntityGraph,
  useEntityHierarchy,
  useLinkManifest,
  useRemoveDownstreamDep,
  useRemoveUpstreamDep,
  useUnlinkManifest,
  type EntityKind,
} from '@/lib/queries/entity'
import type { HierarchyNode, Manifest, Task } from '@/lib/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import { LinkOrCreateModal } from '../link-or-create-modal'
import type { PickerRow } from '../dep-picker'
import '@xyflow/react/dist/style.css'

// useQueryDirection — small kind-agnostic hook for the inverse-edge
// fetch on tasks (and any future kind that needs both directions
// distinct). The generic `useEntityDependencies` is hard-coded to the
// "out" direction so the products + manifests reading paths stay
// stable; this hook is the escape hatch for the DAG tab when it
// needs the "in" direction.
function useQueryDirection(taskId: string | undefined, direction: 'in' | 'out') {
  return useQuery({
    queryKey: ['task', 'deps', taskId ?? '', direction],
    queryFn: async () => {
      const r = await fetch(`/api/tasks/${taskId}/dependencies?direction=${direction}`)
      if (!r.ok) throw new Error(`task deps ${direction} → ${r.status}`)
      const data = (await r.json()) as
        | { deps?: PickerRow[]; dependents?: PickerRow[] }
      const rows = direction === 'in' ? data.dependents : data.deps
      return Array.isArray(rows) ? rows : []
    },
    enabled: !!taskId,
    staleTime: 30 * 1000,
  })
}

// Tailwind-compatible status border colors. Keep the same palette the
// cytoscape renderer used so visual continuity stays intact.
const STATUS_BORDER: Record<string, string> = {
  open: '#10b981',
  in_progress: '#0ea5e9',
  draft: '#f59e0b',
  closed: '#71717a',
  archived: '#52525b',
  cancelled: '#f43f5e',
}

type EntityNodeData = {
  label: string
  status: string
  type: string
  parent_id?: string
  current: boolean
}

// Three custom React Flow node components with distinct shapes —
// product = rounded rectangle (anchor / container), manifest =
// hexagon (specification), task = pill (actionable). Same status
// border palette across all three for visual continuity. Each MUST
// include <Handle> components — without them React Flow renders the
// edge marker defs but not the path connecting source to target.
// LR layout: edges enter from the left, exit on the right.
function NodeHandles() {
  return (
    <>
      <Handle
        type='target'
        position={Position.Left}
        style={{ opacity: 0, pointerEvents: 'none' }}
      />
      <Handle
        type='source'
        position={Position.Right}
        style={{ opacity: 0, pointerEvents: 'none' }}
      />
    </>
  )
}

function NodeBody({ data, children }: { data: EntityNodeData; children?: React.ReactNode }) {
  return (
    <>
      <div className='truncate text-xs font-medium leading-tight'>
        {data.label}
      </div>
      <div className='text-muted-foreground truncate text-[10px] uppercase tracking-wide'>
        {data.type}
      </div>
      {children}
    </>
  )
}

function ProductNode({ data }: NodeProps<Node<EntityNodeData>>) {
  const border = STATUS_BORDER[data.status] ?? '#71717a'
  return (
    <div
      className={
        'bg-card text-foreground rounded-md border-2 px-3 py-1.5 shadow-sm ' +
        (data.current ? 'ring-primary ring-2 ring-offset-1' : '')
      }
      style={{ borderColor: border, width: 180, height: 56 }}
    >
      <NodeHandles />
      <NodeBody data={data} />
    </div>
  )
}

function ManifestNode({ data }: NodeProps<Node<EntityNodeData>>) {
  const border = STATUS_BORDER[data.status] ?? '#71717a'
  const clip =
    'polygon(12% 0, 88% 0, 100% 50%, 88% 100%, 12% 100%, 0 50%)'
  return (
    <div
      className={
        'relative shadow-sm ' + (data.current ? 'ring-primary ring-2' : '')
      }
      style={{ width: 200, height: 64 }}
    >
      <NodeHandles />
      <div
        className='absolute inset-0'
        style={{ background: border, clipPath: clip }}
      />
      <div
        className='bg-card text-foreground absolute flex flex-col justify-center px-4 py-1'
        style={{ inset: 2, clipPath: clip }}
      >
        <NodeBody data={data} />
      </div>
    </div>
  )
}

function TaskNode({ data }: NodeProps<Node<EntityNodeData>>) {
  const border = STATUS_BORDER[data.status] ?? '#71717a'
  return (
    <div
      className={
        'bg-card text-foreground rounded-full border-2 px-4 py-1.5 shadow-sm ' +
        (data.current ? 'ring-primary ring-2 ring-offset-1' : '')
      }
      style={{ borderColor: border, width: 180, height: 56 }}
    >
      <NodeHandles />
      <NodeBody data={data} />
    </div>
  )
}

const NODE_TYPES = {
  entity: ProductNode, // legacy fallback (existing call sites pass type:'entity')
  product: ProductNode,
  manifest: ManifestNode,
  task: TaskNode,
}

// dagre top→bottom layout. Returns a fresh nodes/edges pair with
// computed positions. Nodes are anchor-centered; we shift back by
// half the box so React Flow's top-left convention lines up.
const NODE_W = 170
const NODE_H = 56
const elk = new ELK()

// ELK.js layered layout — handles wide fan-out (umbrella → 20+
// sub-products) gracefully where dagre would explode the sibling row
// across the canvas. The 'layered' algorithm produces the same
// hierarchical look but with smarter edge routing + node ordering.
//
// Direction:
//   - 'RIGHT' (left → right) — vertical stacks per rank; works best
//     for trees with wide siblings.
//   - 'DOWN' (top → bottom) — classic org-chart shape; better for
//     narrow trees.
//
// Returns a Promise — caller awaits or wraps in useState/useEffect.
async function layoutELK(
  nodes: Node[],
  edges: Edge[],
  dir: 'RIGHT' | 'DOWN' = 'RIGHT'
): Promise<{ nodes: Node[]; edges: Edge[] }> {
  if (nodes.length === 0) return { nodes, edges }
  const isHorizontal = dir === 'RIGHT'
  const graph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': dir,
      'elk.layered.spacing.nodeNodeBetweenLayers': '60',
      'elk.spacing.nodeNode': '24',
      'elk.layered.crossingMinimization.semiInteractive': 'true',
      'elk.edgeRouting': 'POLYLINE',
    },
    children: nodes.map((n) => ({
      id: n.id,
      width: NODE_W,
      height: NODE_H,
    })),
    edges: edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  }
  try {
    const laid = await elk.layout(graph)
    const positionMap = new Map<string, { x: number; y: number }>()
    for (const c of laid.children ?? []) {
      positionMap.set(c.id!, { x: c.x ?? 0, y: c.y ?? 0 })
    }
    return {
      nodes: nodes.map((n) => {
        const p = positionMap.get(n.id) ?? { x: 0, y: 0 }
        return {
          ...n,
          position: p,
          targetPosition: isHorizontal ? Position.Left : Position.Top,
          sourcePosition: isHorizontal ? Position.Right : Position.Bottom,
        }
      }),
      edges,
    }
  } catch (err) {
    console.error('elk layout failed', err)
    return { nodes, edges }
  }
}

type GraphInput = { nodes: Node[]; edges: Edge[] }

function pushNode(
  acc: GraphInput,
  id: string,
  data: EntityNodeData
) {
  if (acc.nodes.some((n) => n.id === id)) return
  acc.nodes.push({
    id,
    type: 'entity',
    position: { x: 0, y: 0 },
    data,
    draggable: false,
    selectable: true,
  })
}

// Edge style — SVG <path> doesn't resolve CSS vars on stroke, so use a
// concrete color (matches the muted-foreground token in the dark theme).
// Marker color must be set explicitly too — otherwise the arrowhead
// inherits a default that's often invisible against the canvas.
const EDGE_COLOR = '#94a3b8'

function pushEdge(acc: GraphInput, source: string, target: string) {
  const id = `${source}->${target}`
  if (acc.edges.some((e) => e.id === id)) return
  acc.edges.push({
    id,
    source,
    target,
    type: 'smoothstep',
    animated: false,
    markerEnd: {
      type: MarkerType.ArrowClosed,
      width: 16,
      height: 16,
      color: EDGE_COLOR,
    },
    style: { stroke: EDGE_COLOR, strokeWidth: 1.5 },
  })
}

function productGraph(
  root: HierarchyNode | undefined,
  currentId: string
): GraphInput {
  const acc: GraphInput = { nodes: [], edges: [] }
  if (!root) return acc
  const visit = (n: HierarchyNode, parentId?: string) => {
    pushNode(acc, n.id, {
      label: n.title,
      status: n.status,
      type: n.type,
      parent_id: parentId,
      current: n.id === currentId,
    })
    if (parentId) pushEdge(acc, parentId, n.id)
    const children = [...(n.sub_products ?? []), ...(n.children ?? [])]
    for (const c of children) visit(c, n.id)
  }
  visit(root)
  return acc
}

function taskGraph(
  taskId: string,
  task: Task | undefined,
  upstreamDeps: PickerRow[],
  downstreamDeps: PickerRow[]
): GraphInput {
  const acc: GraphInput = { nodes: [], edges: [] }
  if (!task) return acc
  for (const dep of upstreamDeps) {
    pushNode(acc, dep.id, {
      label: dep.title,
      status: dep.status,
      type: 'task',
      current: false,
    })
    pushEdge(acc, dep.id, taskId)
  }
  pushNode(acc, taskId, {
    label: task.title,
    status: task.status,
    type: 'task',
    current: true,
  })
  for (const d of downstreamDeps) {
    pushNode(acc, d.id, {
      label: d.title,
      status: d.status,
      type: 'task',
      current: false,
    })
    pushEdge(acc, taskId, d.id)
  }
  return acc
}

function manifestGraph(
  manifestId: string,
  manifest: Manifest | undefined,
  parentProductTitle: string | undefined,
  upstreamDeps: PickerRow[],
  childTasks: PickerRow[]
): GraphInput {
  const acc: GraphInput = { nodes: [], edges: [] }
  if (!manifest) return acc
  if (manifest.project_id) {
    pushNode(acc, manifest.project_id, {
      label: parentProductTitle ?? manifest.project_id.slice(0, 12),
      status: 'open',
      type: 'product',
      current: false,
    })
    pushEdge(acc, manifest.project_id, manifestId)
  }
  pushNode(acc, manifestId, {
    label: manifest.title,
    status: manifest.status,
    type: 'manifest',
    parent_id: manifest.project_id || undefined,
    current: true,
  })
  for (const dep of upstreamDeps) {
    pushNode(acc, dep.id, {
      label: dep.title,
      status: dep.status,
      type: 'manifest',
      current: false,
    })
    pushEdge(acc, dep.id, manifestId)
  }
  for (const t of childTasks) {
    pushNode(acc, t.id, {
      label: t.title,
      status: t.status,
      type: 'task',
      parent_id: manifestId,
      current: false,
    })
    pushEdge(acc, manifestId, t.id)
  }
  return acc
}

// Inner component — needs to live inside <ReactFlowProvider> so the
// useReactFlow hook can drive fitView() from the toolbar.
function DAGTabInner({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const isProduct = kind === 'product'
  const isTask = kind === 'task'
  const hierarchy = useEntityHierarchy(kind, entityId)
  const deps = useEntityDependencies(kind, entityId)
  const children = useEntityChildren(kind, entityId)
  const allProducts = useAllProducts()
  const allManifests = useAllManifests()
  const navigate = useNavigate()
  const rf = useReactFlow()

  // Manifest-mode extras: read this manifest + its parent product so
  // we can draw the parent-product crumb-node above the manifest.
  const manifestSelf = useEntity(kind, isProduct || isTask ? undefined : entityId)
  const m = manifestSelf.data as Manifest | undefined
  const parent = useEntity('product', m?.project_id || undefined)

  // Task-mode extras: read this task + the inverse-direction deps.
  const taskSelf = useEntity(kind, isTask ? entityId : undefined)
  const taskData = taskSelf.data as Task | undefined
  const taskDownstream = useQueryDirection(
    isTask ? entityId : undefined,
    'in'
  )

  const [editing, setEditing] = useState(false)
  const [modal, setModal] = useState<
    null | 'subproduct' | 'manifest' | 'manifest-upstream'
  >(null)
  const [connectMode, setConnectMode] = useState(false)
  const [connectFrom, setConnectFrom] = useState<null | {
    id: string
    title: string
    type: string
  }>(null)
  const [connectReverse, setConnectReverse] = useState(false)
  const [edgeConfirm, setEdgeConfirm] = useState<null | {
    fromId: string
    fromTitle: string
    toId: string
    toTitle: string
    reversed: boolean
  }>(null)
  const [unlinkConfirm, setUnlinkConfirm] = useState<null | {
    nodeId: string
    nodeTitle: string
    parentId: string
    nodeType: string
  }>(null)

  const addDownstream = useAddDownstreamDep(kind, entityId)
  const remDownstream = useRemoveDownstreamDep(kind, entityId)
  const addUpstream = useAddUpstreamDep(kind, entityId)
  const remUpstream = useRemoveUpstreamDep(kind, entityId)
  const linkM = useLinkManifest(entityId)
  const unlinkM = useUnlinkManifest(entityId)
  const createSub = useCreateAndLinkSubProduct(entityId)
  const createM = useCreateAndLinkManifest(entityId)
  const createUpstream = useCreateAndLinkUpstreamManifest(
    entityId,
    m?.project_id
  )

  const upstreamRows: PickerRow[] = (deps.data ?? []).map((d) => ({
    id: d.id,
    marker: d.marker,
    title: d.title,
    status: d.status,
  }))
  const childRows: PickerRow[] = (children.data ?? []).map(
    (c: { id: string; marker?: string; title?: string; status?: string }) => ({
      id: c.id,
      marker: c.marker ?? c.id.slice(0, 12),
      title: c.title ?? c.id.slice(0, 12),
      status: c.status ?? '',
    })
  )

  const downstreamTaskRows: PickerRow[] = (taskDownstream.data ?? []).map(
    (d) => ({
      id: d.id,
      marker: d.marker,
      title: d.title,
      status: d.status,
    })
  )

  // Single source of truth: the relationships table. Walk all
  // reachable nodes from this entity, get back a flat (nodes, edges)
  // payload, build React Flow nodes + edges directly. No nested
  // hierarchy walker, no kind-specific JSON shape — the relationships
  // store IS the graph.
  const graphResp = useEntityGraph(kind, entityId, 10)
  const graph = useMemo<GraphInput>(() => {
    const acc: GraphInput = { nodes: [], edges: [] }
    if (!graphResp.data) return acc
    for (const n of graphResp.data.nodes) {
      acc.nodes.push({
        id: n.id,
        type: n.kind, // 'product' | 'manifest' | 'task' → matches NODE_TYPES
        position: { x: 0, y: 0 },
        data: {
          label: n.title,
          status: n.status,
          type: n.kind,
          current: n.id === entityId,
        },
        draggable: false,
        selectable: true,
      })
    }
    for (const e of graphResp.data.edges) {
      // Style edges differently by kind. Owns = solid blue (the
      // "container" relationship); depends_on = dashed amber (the
      // "blocking" relationship). Both arrow-headed.
      const isOwns = e.kind === 'owns'
      acc.edges.push({
        id: e.id,
        source: e.source,
        target: e.target,
        type: 'smoothstep',
        animated: false,
        markerEnd: {
          type: MarkerType.ArrowClosed,
          width: 16,
          height: 16,
          color: isOwns ? '#3b82f6' : '#f59e0b',
        },
        style: {
          stroke: isOwns ? '#3b82f6' : '#f59e0b',
          strokeWidth: 1.5,
          strokeDasharray: isOwns ? undefined : '6 4',
        },
      })
    }
    return acc
  }, [graphResp.data, entityId])

  // ELK.js layout is async. Hold the result in state, recompute on
  // graph change. Initial paint shows nodes at (0,0) for one frame
  // until ELK returns — fine for our graph sizes (typically < 200ms).
  const [laidOut, setLaidOut] = useState<GraphInput>({ nodes: graph.nodes, edges: graph.edges })
  useEffect(() => {
    let cancelled = false
    layoutELK(graph.nodes, graph.edges, 'RIGHT').then((res) => {
      if (cancelled) return
      setLaidOut(res)
    })
    return () => {
      cancelled = true
    }
  }, [graph])
  // Refit whenever the graph changes — same UX as cytoscape's
  // layout.fit:true on init.
  useEffect(() => {
    if (laidOut.nodes.length === 0) return
    const t = window.setTimeout(() => {
      rf.fitView({ padding: 0.15, duration: 250 })
    }, 50)
    return () => window.clearTimeout(t)
  }, [laidOut, rf])

  const snapshot = useMemo(
    () => ({
      upstream: isProduct ? [] : upstreamRows.map((r) => r.id),
      downstream: isProduct ? upstreamRows.map((r) => r.id) : [],
      manifests: isProduct ? childRows.map((r) => r.id) : [],
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isProduct, deps.data, children.data]
  )

  const subProductCandidates = useMemo<PickerRow[]>(() => {
    const exclude = new Set([entityId, ...upstreamRows.map((r) => r.id)])
    return (allProducts.data ?? [])
      .filter((p) => !exclude.has(p.id))
      .map((p) => ({
        id: p.id,
        marker: p.marker,
        title: p.title,
        status: p.status,
      }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allProducts.data, deps.data, entityId])

  const manifestLinkCandidates = useMemo<PickerRow[]>(() => {
    const exclude = new Set(childRows.map((r) => r.id))
    return (allManifests.data ?? [])
      .filter((mm) => !exclude.has(mm.id))
      .map((mm) => ({
        id: mm.id,
        marker: mm.marker,
        title: mm.title,
        status: mm.status,
      }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allManifests.data, children.data])

  const manifestUpstreamCandidates = useMemo<PickerRow[]>(() => {
    const exclude = new Set([entityId, ...upstreamRows.map((r) => r.id)])
    return (allManifests.data ?? [])
      .filter((mm) => !exclude.has(mm.id))
      .map((mm) => ({
        id: mm.id,
        marker: mm.marker,
        title: mm.title,
        status: mm.status,
      }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allManifests.data, deps.data, entityId])

  const onNodeClick = (
    _e: React.MouseEvent,
    node: Node<EntityNodeData>
  ) => {
    if (editing && connectMode) {
      const { id } = node
      const title = node.data.label
      const type = node.data.type
      if (!connectFrom) {
        setConnectFrom({ id, title, type })
        return
      }
      if (connectFrom.id === id) return
      setEdgeConfirm({
        fromId: connectFrom.id,
        fromTitle: connectFrom.title,
        toId: id,
        toTitle: title,
        reversed: connectReverse,
      })
      setConnectFrom(null)
      return
    }
    if (node.id === entityId) return
    if (editing) return
    const type = node.data.type
    if (type === 'product') {
      navigate({ to: '/products', search: { id: node.id, tab: 'dag' } })
    } else if (type === 'manifest') {
      navigate({ to: '/manifests', search: { id: node.id, tab: 'dag' } })
    } else if (type === 'task') {
      navigate({ to: '/tasks', search: { id: node.id, tab: 'dag' } })
    }
  }

  const onNodeContextMenu = (
    e: React.MouseEvent,
    node: Node<EntityNodeData>
  ) => {
    if (!editing) return
    e.preventDefault()
    if (node.id === entityId) return
    const parentId = node.data.parent_id
    const type = node.data.type
    if (!parentId && type !== 'manifest') return
    if (isProduct && parentId !== entityId) {
      toast.message('Drill into the parent to edit its children.')
      return
    }
    setUnlinkConfirm({
      nodeId: node.id,
      nodeTitle: node.data.label,
      parentId: parentId ?? entityId,
      nodeType: type,
    })
  }

  const onEdgeConfirmed = async () => {
    if (!edgeConfirm) return
    const { fromId, fromTitle, toId, toTitle, reversed } = edgeConfirm
    const srcId = reversed ? toId : fromId
    const dstId = reversed ? fromId : toId
    const srcTitle = reversed ? toTitle : fromTitle
    const dstTitle = reversed ? fromTitle : toTitle
    try {
      const path = isProduct
        ? `/api/products/${srcId}/dependencies`
        : `/api/manifests/${srcId}/dependencies`
      const r = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ depends_on_id: dstId }),
      })
      if (!r.ok && r.status !== 409) {
        throw new Error(`HTTP ${r.status}`)
      }
      toast.success(`Edge added — "${srcTitle}" depends on "${dstTitle}"`)
      await hierarchy.refetch?.()
      await deps.refetch()
      await children.refetch()
    } catch (e) {
      toast.error(`Failed: ${String(e)}`)
    }
    setEdgeConfirm(null)
  }

  const onUnlinkConfirmed = async () => {
    if (!unlinkConfirm) return
    try {
      const target = {
        id: unlinkConfirm.nodeId,
        marker: unlinkConfirm.nodeId.slice(0, 12),
        title: unlinkConfirm.nodeTitle,
      }
      if (isProduct) {
        if (unlinkConfirm.nodeType === 'manifest') {
          await unlinkM.mutateAsync({ target, snapshot })
          toast.success(`Unlinked manifest "${unlinkConfirm.nodeTitle}"`)
        } else {
          await remDownstream.mutateAsync({ target, snapshot })
          toast.success(`Unlinked sub-product "${unlinkConfirm.nodeTitle}"`)
        }
      } else {
        await remUpstream.mutateAsync({ target, snapshot })
        toast.success(`Removed upstream "${unlinkConfirm.nodeTitle}"`)
      }
    } catch (e) {
      toast.error(`Failed: ${String(e)}`)
    }
    setUnlinkConfirm(null)
  }

  const isLoading = isProduct
    ? hierarchy.isLoading
    : isTask
      ? taskSelf.isLoading
      : manifestSelf.isLoading
  const isError = isProduct
    ? hierarchy.isError || !hierarchy.data
    : isTask
      ? taskSelf.isError || !taskSelf.data
      : manifestSelf.isError || !manifestSelf.data

  if (isLoading) {
    return (
      <Card>
        <CardContent className='p-3'>
          <Skeleton className='h-96 w-full' />
        </CardContent>
      </Card>
    )
  }
  if (isError) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load graph:{' '}
        {String(hierarchy.error ?? manifestSelf.error ?? 'no data')}
      </div>
    )
  }

  const hasGraph = laidOut.nodes.length > 0

  return (
    <Card className='gap-0 py-0'>
      <CardContent className='relative p-0'>
        {hasGraph ? (
          <div
            className='bg-background h-[calc(100vh-15rem)] min-h-[600px] w-full rounded-md'
            data-testid='rf__wrapper'
          >
            <ReactFlow
              nodes={laidOut.nodes}
              edges={laidOut.edges}
              nodeTypes={NODE_TYPES}
              onNodeClick={onNodeClick}
              onNodeContextMenu={onNodeContextMenu}
              nodesDraggable={false}
              nodesConnectable={false}
              elementsSelectable={true}
              fitView
              fitViewOptions={{ padding: 0.15 }}
              minZoom={0.1}
              maxZoom={2.5}
              proOptions={{ hideAttribution: true }}
            >
              <Background gap={16} size={1} />
              <Controls position='bottom-left' showInteractive={false} />
              <MiniMap pannable zoomable position='bottom-right' />
            </ReactFlow>
          </div>
        ) : (
          <div className='text-muted-foreground p-6 text-sm'>
            No graph yet. Click <Pencil className='inline h-3 w-3' /> Edit
            to add connections.
          </div>
        )}

        <div className='absolute top-2 right-2 z-10 flex items-center gap-1.5'>
          {editing ? (
            <>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant='outline'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    disabled={connectMode}
                  >
                    <Plus className='mr-1 h-3 w-3' />
                    Add
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end'>
                  {isProduct ? (
                    <>
                      <DropdownMenuItem onClick={() => setModal('subproduct')}>
                        Product (sub-product)
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setModal('manifest')}>
                        Manifest
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled>
                        Task (use Tasks menu)
                      </DropdownMenuItem>
                    </>
                  ) : isTask ? (
                    <>
                      <DropdownMenuItem disabled>
                        Sub-product (n/a — tasks are leaves)
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled>
                        Manifest (n/a)
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled>
                        Task (use Tasks menu)
                      </DropdownMenuItem>
                    </>
                  ) : (
                    <>
                      <DropdownMenuItem
                        onClick={() => setModal('manifest-upstream')}
                      >
                        Manifest (dependency)
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled>
                        Task (use Tasks menu)
                      </DropdownMenuItem>
                    </>
                  )}
                  <DropdownMenuItem
                    onClick={() => {
                      setConnectMode(true)
                      setConnectFrom(null)
                      setConnectReverse(false)
                    }}
                  >
                    Relationship (tap-to-edge)
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <Button
                variant='secondary'
                size='sm'
                className='h-7 px-2 text-xs'
                onClick={() => {
                  setEditing(false)
                  setConnectMode(false)
                  setConnectFrom(null)
                }}
              >
                <X className='mr-1 h-3 w-3' />
                Done
              </Button>
            </>
          ) : (
            <Button
              variant='outline'
              size='sm'
              className='h-7 px-2 text-xs'
              onClick={() => setEditing(true)}
            >
              <Pencil className='mr-1 h-3 w-3' />
              Edit
            </Button>
          )}
          <Button
            variant='outline'
            size='sm'
            className='h-7 px-2 text-xs'
            onClick={() => rf.fitView({ padding: 0.15, duration: 250 })}
            disabled={!hasGraph}
          >
            Fit
          </Button>
        </div>

        {connectMode ? (
          <div className='bg-card/95 absolute bottom-2 left-1/2 z-10 flex -translate-x-1/2 items-center gap-2 rounded-md border px-3 py-1.5 text-xs backdrop-blur'>
            <span>
              {connectFrom
                ? `From "${connectFrom.title}" — click target node`
                : 'Click source node, then target.'}
            </span>
            <Button
              variant='ghost'
              size='sm'
              className='h-6 px-2 text-[11px]'
              onClick={() => setConnectReverse((v) => !v)}
            >
              Direction: {connectReverse ? 'target → source' : 'source → target'}{' '}
              ↔
            </Button>
            <Button
              variant='ghost'
              size='sm'
              className='h-6 px-2 text-[11px]'
              onClick={() => {
                setConnectMode(false)
                setConnectFrom(null)
              }}
            >
              Cancel
            </Button>
          </div>
        ) : editing ? (
          <div className='bg-card/90 absolute bottom-2 left-1/2 z-10 -translate-x-1/2 rounded-md border px-3 py-1.5 text-xs backdrop-blur'>
            Right-click a child node to unlink. Pick "Add" → Relationship
            to draw an edge.
          </div>
        ) : null}

        {isProduct ? (
          <>
            <LinkOrCreateModal
              open={modal === 'subproduct'}
              onOpenChange={(open) => setModal(open ? 'subproduct' : null)}
              title='Sub-product'
              description='Pull an existing product in as a sub-product, or create a new one and link it.'
              candidates={subProductCandidates}
              loading={allProducts.isLoading}
              createLabel='Create + link'
              onLinkExisting={async (row) => {
                try {
                  await addDownstream.mutateAsync({ target: row, snapshot })
                  toast.success(`Linked "${row.title}" as sub-product`)
                } catch (e) {
                  toast.error(`Failed: ${String(e)}`)
                }
              }}
              onCreateNew={async (title) => {
                try {
                  const c = await createSub.mutateAsync({ title, snapshot })
                  toast.success(`Created + linked "${c.title}"`)
                } catch (e) {
                  toast.error(`Failed: ${String(e)}`)
                }
              }}
            />
            <LinkOrCreateModal
              open={modal === 'manifest'}
              onOpenChange={(open) => setModal(open ? 'manifest' : null)}
              title='Manifest'
              description='Pull an existing manifest under this product, or create a new one.'
              candidates={manifestLinkCandidates}
              loading={allManifests.isLoading}
              createLabel='Create + link'
              onLinkExisting={async (row) => {
                try {
                  await linkM.mutateAsync({ target: row, snapshot })
                  toast.success(`Linked manifest "${row.title}"`)
                } catch (e) {
                  toast.error(`Failed: ${String(e)}`)
                }
              }}
              onCreateNew={async (title) => {
                try {
                  const c = await createM.mutateAsync({ title, snapshot })
                  toast.success(`Created + linked "${c.title}"`)
                } catch (e) {
                  toast.error(`Failed: ${String(e)}`)
                }
              }}
            />
          </>
        ) : isTask ? null : (
          <LinkOrCreateModal
            open={modal === 'manifest-upstream'}
            onOpenChange={(open) =>
              setModal(open ? 'manifest-upstream' : null)
            }
            title='Manifest (dependency)'
            description='Pick a manifest this one depends on, or create a new one. The new manifest inherits the parent product.'
            candidates={manifestUpstreamCandidates}
            loading={allManifests.isLoading}
            createLabel='Create + link'
            onLinkExisting={async (row) => {
              try {
                await addUpstream.mutateAsync({ target: row, snapshot })
                toast.success(`Added upstream "${row.title}"`)
              } catch (e) {
                toast.error(`Failed: ${String(e)}`)
              }
            }}
            onCreateNew={async (title) => {
              try {
                const c = await createUpstream.mutateAsync({
                  title,
                  snapshot,
                })
                toast.success(`Created + linked "${c.title}"`)
              } catch (e) {
                toast.error(`Failed: ${String(e)}`)
              }
            }}
          />
        )}

        <AlertDialog
          open={unlinkConfirm !== null}
          onOpenChange={(open) => !open && setUnlinkConfirm(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                Unlink {unlinkConfirm?.nodeType ?? 'node'}?
              </AlertDialogTitle>
              <AlertDialogDescription>
                Sever the link to{' '}
                <span className='font-medium'>
                  {unlinkConfirm?.nodeTitle}
                </span>
                . The entity stays alive as standalone — you can re-link
                it later. Action is logged in revision history.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={onUnlinkConfirmed}>
                <Unlink className='mr-1 h-3 w-3' />
                Unlink
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <AlertDialog
          open={edgeConfirm !== null}
          onOpenChange={(open) => !open && setEdgeConfirm(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Add relationship?</AlertDialogTitle>
              <AlertDialogDescription>
                {edgeConfirm ? (
                  <>
                    <span className='font-medium'>
                      {edgeConfirm.reversed
                        ? edgeConfirm.toTitle
                        : edgeConfirm.fromTitle}
                    </span>
                    {' depends on '}
                    <span className='font-medium'>
                      {edgeConfirm.reversed
                        ? edgeConfirm.fromTitle
                        : edgeConfirm.toTitle}
                    </span>
                    .
                  </>
                ) : null}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={onEdgeConfirmed}>
                Add edge
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  )
}

// Provider wrapper — useReactFlow() needs a provider above the component
// using it. Wrapping the whole tab keeps the API unchanged for callers.
export function DAGTab({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  return (
    <ReactFlowProvider>
      <DAGTabInner kind={kind} entityId={entityId} />
    </ReactFlowProvider>
  )
}
