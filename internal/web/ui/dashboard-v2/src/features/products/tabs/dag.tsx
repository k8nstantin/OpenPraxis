import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import cytoscape from 'cytoscape'
// @ts-expect-error — cytoscape-dagre ships without bundled types
import dagre from 'cytoscape-dagre'
import { Pencil, Plus, Unlink, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  useAddDownstreamProductDep,
  useAllManifests,
  useCreateAndLinkManifest,
  useCreateAndLinkSubProduct,
  useLinkManifest,
  useProductDependencies,
  useProductHierarchy,
  useProductManifests,
  useProducts,
  useRemoveDownstreamProductDep,
  useUnlinkManifest,
} from '@/lib/queries/products'
import type { HierarchyNode } from '@/lib/types'
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

function toElements(root: HierarchyNode | undefined): CyEl[] {
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

export function DAGTab({ productId }: { productId: string }) {
  const hierarchy = useProductHierarchy(productId)
  const subs = useProductDependencies(productId)
  const manifests = useProductManifests(productId)
  const allProducts = useProducts()
  const allManifests = useAllManifests()

  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<cytoscape.Core | null>(null)
  const navigate = useNavigate()

  const [editing, setEditing] = useState(false)
  const [modal, setModal] = useState<null | 'subproduct' | 'manifest'>(null)
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
    isManifest: boolean
  }>(null)

  const addSub = useAddDownstreamProductDep(productId)
  const remSub = useRemoveDownstreamProductDep(productId)
  const linkM = useLinkManifest(productId)
  const unlinkM = useUnlinkManifest(productId)
  const createSub = useCreateAndLinkSubProduct(productId)
  const createM = useCreateAndLinkManifest(productId)

  const elements = useMemo(() => toElements(hierarchy.data), [hierarchy.data])

  const snapshot = useMemo(
    () => ({
      upstream: [],
      downstream: (subs.data ?? []).map((d) => d.id),
      manifests: (manifests.data ?? []).map((m) => m.id),
    }),
    [subs.data, manifests.data]
  )

  const subProductCandidates = useMemo<PickerRow[]>(() => {
    const exclude = new Set([productId, ...(subs.data ?? []).map((d) => d.id)])
    return (allProducts.data ?? [])
      .filter((p) => !exclude.has(p.id))
      .map((p) => ({
        id: p.id,
        marker: p.marker,
        title: p.title,
        status: p.status,
      }))
  }, [allProducts.data, subs.data, productId])

  const manifestCandidates = useMemo<PickerRow[]>(() => {
    const exclude = new Set((manifests.data ?? []).map((m) => m.id))
    return (allManifests.data ?? [])
      .filter((m) => !exclude.has(m.id))
      .map((m) => ({
        id: m.id,
        marker: m.marker,
        title: m.title,
        status: m.status,
      }))
  }, [allManifests.data, manifests.data])

  // Stash editing + connect-mode state in refs so cytoscape event
  // handlers (closed over initial values at registration) read live
  // state without forcing a canvas re-init on every toggle.
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
          selector: `node[id = "${productId}"]`,
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
      // Connect mode short-circuits everything else: stage source on
      // first tap, stage target + open confirm on second.
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
      if (id === productId) return
      if (editingRef.current) return
      navigate({ to: '/products', search: { id, tab: 'dag' } })
    })

    cy.on('cxttap', 'node', (e) => {
      if (!editingRef.current) return
      const node = e.target as cytoscape.NodeSingular
      const id = node.id()
      if (id === productId) return
      const parentId = node.data('parent_id') as string | undefined
      const type = node.data('type') as string
      if (!parentId) return
      if (parentId !== productId) {
        toast.message('Drill into the parent to edit its children.')
        return
      }
      setUnlinkConfirm({
        nodeId: id,
        nodeTitle: node.data('label') as string,
        parentId,
        isManifest: type === 'manifest',
      })
    })

    cyRef.current = cy
    return () => {
      cy.destroy()
      cyRef.current = null
    }
  }, [elements, productId, navigate])

  // Connect-mode commit: POST a product dependency directly. The
  // arrow points "depends-on": source depends on target. When the
  // operator flipped the direction toggle, swap before sending.
  const onEdgeConfirmed = async () => {
    if (!edgeConfirm) return
    const { fromId, fromTitle, toId, toTitle, reversed } = edgeConfirm
    const srcId = reversed ? toId : fromId
    const dstId = reversed ? fromId : toId
    const srcTitle = reversed ? toTitle : fromTitle
    const dstTitle = reversed ? fromTitle : toTitle
    try {
      const r = await fetch(`/api/products/${srcId}/dependencies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ depends_on_id: dstId }),
      })
      if (!r.ok && r.status !== 409) {
        throw new Error(`HTTP ${r.status}`)
      }
      toast.success(
        `Edge added — "${srcTitle}" depends on "${dstTitle}"`
      )
      // Hierarchy refetch picks up the new edge.
      await hierarchy.refetch()
      await subs.refetch()
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
      if (unlinkConfirm.isManifest) {
        await unlinkM.mutateAsync({ target, snapshot })
        toast.success(`Unlinked manifest "${unlinkConfirm.nodeTitle}"`)
      } else {
        await remSub.mutateAsync({ target, snapshot })
        toast.success(`Unlinked sub-product "${unlinkConfirm.nodeTitle}"`)
      }
    } catch (e) {
      toast.error(`Failed: ${String(e)}`)
    }
    setUnlinkConfirm(null)
  }

  if (hierarchy.isLoading) {
    return (
      <Card>
        <CardContent className='p-3'>
          <Skeleton className='h-96 w-full' />
        </CardContent>
      </Card>
    )
  }
  if (hierarchy.isError || !hierarchy.data) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load hierarchy: {String(hierarchy.error ?? 'no data')}
      </div>
    )
  }

  return (
    <Card className='gap-0 py-0'>
      <CardContent className='relative p-0'>
        {elements.length === 0 ? (
          <div className='text-muted-foreground p-6 text-sm'>
            No graph yet — this product has no sub-products. Click{' '}
            <Pencil className='inline h-3 w-3' /> Edit to add children.
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
                  <DropdownMenuItem onClick={() => setModal('subproduct')}>
                    Product (sub-product)
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setModal('manifest')}>
                    Manifest
                  </DropdownMenuItem>
                  <DropdownMenuItem disabled>
                    Task (pending Tasks menu)
                  </DropdownMenuItem>
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
              await addSub.mutateAsync({ target: row, snapshot })
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
          candidates={manifestCandidates}
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

        <AlertDialog
          open={unlinkConfirm !== null}
          onOpenChange={(open) => !open && setUnlinkConfirm(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                Unlink {unlinkConfirm?.isManifest ? 'manifest' : 'sub-product'}
                ?
              </AlertDialogTitle>
              <AlertDialogDescription>
                Sever the link from this product to{' '}
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
                    . Currently only product → product edges are
                    supported here; other combinations land with the
                    Manifest / Task menus.
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
