import { useEffect, useMemo, useRef } from 'react'
import { useNavigate } from '@tanstack/react-router'
import cytoscape from 'cytoscape'
// @ts-expect-error — cytoscape-dagre ships without bundled types
import dagre from 'cytoscape-dagre'
import { useProductHierarchy } from '@/lib/queries/products'
import type { HierarchyNode } from '@/lib/types'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Same library Portal A uses (cytoscape + cytoscape-dagre, vendor-
// pinned). React mount via useEffect, layout via dagre, tap-to-navigate
// for sub-products. No new mental model — operators see the same graph
// shape they're used to.

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
  | { data: { id: string; label: string; status: string; type: string } }
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
  const containerRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<cytoscape.Core | null>(null)
  const navigate = useNavigate()

  const elements = useMemo(() => toElements(hierarchy.data), [hierarchy.data])

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
      minZoom: 0.3,
      maxZoom: 2.5,
      wheelSensitivity: 0.2,
    })

    cy.on('tap', 'node', (e) => {
      const id = e.target.id() as string
      if (id === productId) return
      navigate({ to: '/products', search: { id, tab: 'dag' } })
    })

    cyRef.current = cy
    return () => {
      cy.destroy()
      cyRef.current = null
    }
  }, [elements, productId, navigate])

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
  if (elements.length === 0) {
    return (
      <Card>
        <CardContent className='text-muted-foreground p-6 text-sm'>
          No graph — this product has no sub-products yet.
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardContent className='p-0'>
        <div
          ref={containerRef}
          className='bg-background h-[calc(100vh-22rem)] min-h-96 w-full rounded-md'
        />
      </CardContent>
    </Card>
  )
}
