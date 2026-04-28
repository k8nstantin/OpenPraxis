import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import cytoscape from 'cytoscape'
// @ts-expect-error — cytoscape-dagre ships without bundled types
import dagre from 'cytoscape-dagre'
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

let DAGRE_REGISTERED = false
function ensureDagre() {
  if (DAGRE_REGISTERED) return
  cytoscape.use(dagre)
  DAGRE_REGISTERED = true
}

const STATUS_BORDER: Record<string, string> = {
  open: '#10b981',
  in_progress: '#0ea5e9',
  draft: '#f59e0b',
  closed: '#71717a',
  archived: '#52525b',
  cancelled: '#f43f5e',
}

type CyEl =
  | {
      data: {
        id: string
        label: string
        status: string
        type: string
        parent_id?: string
      }
    }
  | { data: { id: string; source: string; target: string } }

function productElements(root: HierarchyNode | undefined): CyEl[] {
  if (!root) return []
  const els: CyEl[] = []
  const visit = (n: HierarchyNode, parentId?: string) => {
    els.push({
      data: {
        id: n.id,
        label: n.title,
        status: n.status,
        type: n.type,
        parent_id: parentId,
      },
    })
    if (parentId) {
      els.push({
        data: { id: `${parentId}->${n.id}`, source: parentId, target: n.id },
      })
    }
    const children = [...(n.sub_products ?? []), ...(n.children ?? [])]
    for (const c of children) visit(c, n.id)
  }
  visit(root)
  return els
}

// Tasks are leaves in the product-manifest-task tree: one task + its
// upstream + downstream dep edges. No children. Tasks have at most one
// upstream dep on the wire (tasks.depends_on is a single value), but
// the function handles N for symmetry.
function taskElements(
  taskId: string,
  task: Task | undefined,
  upstreamDeps: PickerRow[],
  downstreamDeps: PickerRow[]
): CyEl[] {
  if (!task) return []
  const els: CyEl[] = []
  for (const dep of upstreamDeps) {
    els.push({
      data: { id: dep.id, label: dep.title, status: dep.status, type: 'task' },
    })
    els.push({
      data: { id: `${dep.id}->${taskId}`, source: dep.id, target: taskId },
    })
  }
  els.push({
    data: { id: taskId, label: task.title, status: task.status, type: 'task' },
  })
  for (const d of downstreamDeps) {
    els.push({
      data: { id: d.id, label: d.title, status: d.status, type: 'task' },
    })
    els.push({
      data: { id: `${taskId}->${d.id}`, source: taskId, target: d.id },
    })
  }
  return els
}

// Manifests are flat in the product-manifest-task tree: one manifest +
// optional parent product + its child tasks + its upstream dep edges.
function manifestElements(
  manifestId: string,
  manifest: Manifest | undefined,
  parentProductTitle: string | undefined,
  upstreamDeps: PickerRow[],
  childTasks: PickerRow[]
): CyEl[] {
  if (!manifest) return []
  const els: CyEl[] = []
  // Optional parent product node — drawn above the manifest.
  if (manifest.project_id) {
    els.push({
      data: {
        id: manifest.project_id,
        label: parentProductTitle ?? manifest.project_id.slice(0, 12),
        status: 'open',
        type: 'product',
      },
    })
    els.push({
      data: {
        id: `${manifest.project_id}->${manifestId}`,
        source: manifest.project_id,
        target: manifestId,
      },
    })
  }
  // Center node — this manifest.
  els.push({
    data: {
      id: manifestId,
      label: manifest.title,
      status: manifest.status,
      type: 'manifest',
      parent_id: manifest.project_id || undefined,
    },
  })
  // Upstream deps. Edge "this depends on dep" — arrow points dep → this
  // (dep is a parent in the dagre layout).
  for (const dep of upstreamDeps) {
    els.push({
      data: {
        id: dep.id,
        label: dep.title,
        status: dep.status,
        type: 'manifest',
      },
    })
    els.push({
      data: {
        id: `${dep.id}->${manifestId}`,
        source: dep.id,
        target: manifestId,
      },
    })
  }
  // Child tasks below the manifest.
  for (const t of childTasks) {
    els.push({
      data: {
        id: t.id,
        label: t.title,
        status: t.status,
        type: 'task',
        parent_id: manifestId,
      },
    })
    els.push({
      data: {
        id: `${manifestId}->${t.id}`,
        source: manifestId,
        target: t.id,
      },
    })
  }
  return els
}

export function DAGTab({
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

  // Manifest-mode extras: read this manifest + its parent product so
  // we can draw the parent-product crumb-node above the manifest in
  // the cytoscape graph.
  const manifestSelf = useEntity(kind, isProduct || isTask ? undefined : entityId)
  const m = manifestSelf.data as Manifest | undefined
  const parent = useEntity('product', m?.project_id || undefined)

  // Task-mode extras: read this task + the inverse-direction deps
  // (tasks that depend on this) so the DAG can show both edges.
  const taskSelf = useEntity(kind, isTask ? entityId : undefined)
  const taskData = taskSelf.data as Task | undefined
  // Downstream tasks — fetched separately because useEntityDependencies
  // is hard-coded to direction=out for the generic surface. Same wire
  // shape ({deps: [...]}) the manifest 'in' direction uses.
  const taskDownstream = useQueryDirection(
    isTask ? entityId : undefined,
    'in'
  )

  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<cytoscape.Core | null>(null)

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

  const elements = useMemo(() => {
    if (isProduct) return productElements(hierarchy.data)
    if (isTask)
      return taskElements(entityId, taskData, upstreamRows, downstreamTaskRows)
    return manifestElements(
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

  const editingRef = useRef(editing)
  const connectModeRef = useRef(connectMode)
  const connectFromRef = useRef(connectFrom)
  const connectReverseRef = useRef(connectReverse)
  useEffect(() => {
    editingRef.current = editing
  }, [editing])
  useEffect(() => {
    connectModeRef.current = connectMode
  }, [connectMode])
  useEffect(() => {
    connectFromRef.current = connectFrom
  }, [connectFrom])
  useEffect(() => {
    connectReverseRef.current = connectReverse
  }, [connectReverse])

  useEffect(() => {
    if (!containerRef.current) return
    ensureDagre()

    if (cyRef.current) {
      cyRef.current.destroy()
      cyRef.current = null
    }
    if (elements.length === 0) return

    const cy = cytoscape({
      container: containerRef.current,
      elements,
      layout: {
        name: 'dagre',
        // @ts-expect-error — dagre layout extension props
        rankDir: 'TB',
        nodeSep: 40,
        rankSep: 90,
        edgeSep: 25,
        padding: 32,
        fit: true,
      },
      style: [
        {
          selector: 'node',
          style: {
            label: 'data(label)',
            'text-wrap': 'wrap',
            'text-max-width': '110px',
            'font-size': '9px',
            'text-valign': 'center',
            'text-halign': 'center',
            color: '#e4e4e7',
            'background-color': '#1a1a2e',
            'border-width': 2,
            'border-color': (ele: cytoscape.NodeSingular) =>
              STATUS_BORDER[ele.data('status') as string] ?? '#71717a',
            width: 120,
            height: 44,
            shape: 'round-rectangle',
          },
        },
        {
          selector: 'edge',
          style: {
            width: 1,
            'line-color': '#52525b',
            'target-arrow-color': '#52525b',
            'target-arrow-shape': 'triangle',
            'curve-style': 'bezier',
          },
        },
        {
          selector: `node[id = "${entityId}"]`,
          style: {
            'background-color': '#312e81',
            'border-color': '#a78bfa',
            'border-width': 3,
          },
        },
      ],
      minZoom: 0.05,
      maxZoom: 2.5,
      wheelSensitivity: 0.2,
    })

    cy.on('tap', 'node', (e) => {
      const node = e.target as cytoscape.NodeSingular
      const id = node.id() as string
      if (editingRef.current && connectModeRef.current) {
        const title = node.data('label') as string
        const type = node.data('type') as string
        const cur = connectFromRef.current
        if (!cur) {
          setConnectFrom({ id, title, type })
          return
        }
        if (cur.id === id) return
        const reversed = connectReverseRef.current
        setEdgeConfirm({
          fromId: cur.id,
          fromTitle: cur.title,
          toId: id,
          toTitle: title,
          reversed,
        })
        setConnectFrom(null)
        return
      }
      if (id === entityId) return
      if (editingRef.current) return
      // Navigate based on node type.
      const type = node.data('type') as string
      if (type === 'product') {
        navigate({ to: '/products', search: { id, tab: 'dag' } })
      } else if (type === 'manifest') {
        navigate({ to: '/manifests', search: { id, tab: 'dag' } })
      } else if (type === 'task') {
        navigate({ to: '/tasks', search: { id, tab: 'dag' } })
      }
    })

    cy.on('cxttap', 'node', (e) => {
      if (!editingRef.current) return
      const node = e.target as cytoscape.NodeSingular
      const id = node.id()
      if (id === entityId) return
      const parentId = node.data('parent_id') as string | undefined
      const type = node.data('type') as string
      if (!parentId && type !== 'manifest') return
      // For products: only allow unlinking direct children of THIS.
      if (isProduct && parentId !== entityId) {
        toast.message('Drill into the parent to edit its children.')
        return
      }
      setUnlinkConfirm({
        nodeId: id,
        nodeTitle: node.data('label') as string,
        parentId: parentId ?? entityId,
        nodeType: type,
      })
    })

    cyRef.current = cy
    return () => {
      cy.destroy()
      cyRef.current = null
    }
  }, [elements, entityId, navigate, isProduct])

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
        // Manifest mode — unlink an upstream dep.
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

  return (
    <Card className='gap-0 py-0'>
      <CardContent className='relative p-0'>
        {elements.length === 0 ? (
          <div className='text-muted-foreground p-6 text-sm'>
            No graph yet. Click{' '}
            <Pencil className='inline h-3 w-3' /> Edit to add connections.
          </div>
        ) : (
          <div
            ref={containerRef}
            className='bg-background h-[calc(100vh-15rem)] min-h-[600px] w-full rounded-md'
          />
        )}

        <div className='absolute top-2 right-2 flex items-center gap-1.5'>
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
                    // Tasks own no entities — only the relationship
                    // editor is enabled. Sub-product / Manifest / Task
                    // creation off a task is meaningless in the data
                    // model so the items stay disabled.
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
                    Relationship (drag-to-edge)
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
            onClick={() => cyRef.current?.fit(undefined, 32)}
            disabled={elements.length === 0}
          >
            Fit
          </Button>
        </div>

        {connectMode ? (
          <div className='bg-card/95 absolute bottom-2 left-2 flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs backdrop-blur'>
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
          <div className='bg-card/90 absolute bottom-2 left-2 rounded-md border px-3 py-1.5 text-xs backdrop-blur'>
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
