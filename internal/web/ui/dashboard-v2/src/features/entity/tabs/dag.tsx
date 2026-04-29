import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import dagre from '@dagrejs/dagre'
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

// Custom React Flow node — shadcn-admin themed. Border tints by
// status; ring-2 on the currently-viewed entity ("you are here").
function EntityNode({ data }: NodeProps<Node<EntityNodeData>>) {
  const border = STATUS_BORDER[data.status] ?? '#71717a'
  return (
    <div
      className={
        'bg-card text-foreground rounded-md border-2 px-3 py-1.5 shadow-sm ' +
        (data.current ? 'ring-primary ring-2 ring-offset-1' : '')
      }
      style={{ borderColor: border, width: 180, height: 56 }}
    >
      <div className='truncate text-xs font-medium leading-tight'>
        {data.label}
      </div>
      <div className='text-muted-foreground truncate text-[10px] uppercase tracking-wide'>
        {data.type}
      </div>
    </div>
  )
}

const NODE_TYPES = { entity: EntityNode }

// dagre top→bottom layout. Returns a fresh nodes/edges pair with
// computed positions. Nodes are anchor-centered; we shift back by
// half the box so React Flow's top-left convention lines up.
const NODE_W = 180
const NODE_H = 56
function layout(nodes: Node[], edges: Edge[], dir: 'TB' | 'LR' = 'TB') {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: dir, nodesep: 60, ranksep: 80 })
  g.setDefaultEdgeLabel(() => ({}))
  for (const n of nodes) g.setNode(n.id, { width: NODE_W, height: NODE_H })
  for (const e of edges) g.setEdge(e.source, e.target)
  dagre.layout(g)
  return {
    nodes: nodes.map((n) => {
      const p = g.node(n.id)
      return {
        ...n,
        position: { x: p.x - NODE_W / 2, y: p.y - NODE_H / 2 },
      }
    }),
    edges,
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

function pushEdge(acc: GraphInput, source: string, target: string) {
  const id = `${source}->${target}`
  if (acc.edges.some((e) => e.id === id)) return
  acc.edges.push({
    id,
    source,
    target,
    type: 'smoothstep',
    markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
    style: { stroke: 'var(--muted-foreground)', strokeWidth: 1.25 },
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

  const graph = useMemo<GraphInput>(() => {
    if (isProduct) return productGraph(hierarchy.data, entityId)
    if (isTask)
      return taskGraph(entityId, taskData, upstreamRows, downstreamTaskRows)
    return manifestGraph(
      entityId,
      m,
      parent.data?.title,
      upstreamRows,
      childRows
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    isProduct,
    isTask,
    hierarchy.data,
    entityId,
    m,
    taskData,
    parent.data?.title,
    deps.data,
    children.data,
    taskDownstream.data,
  ])

  const laidOut = useMemo(
    () => layout(graph.nodes, graph.edges, 'TB'),
    [graph]
  )

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
