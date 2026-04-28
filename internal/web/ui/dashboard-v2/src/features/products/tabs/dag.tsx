import { Link } from '@tanstack/react-router'
import { useProductHierarchy } from '@/lib/queries/products'
import type { HierarchyNode } from '@/lib/types'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

// DAG tab — real graph data, indented text rendering. Each row is the
// product's hierarchy walked breadth-first, indented per level. Nodes
// are clickable Links so the operator can jump to any descendant.
//
// The visual canvas (cytoscape pan/zoom) lands in a follow-up — for
// now this text layout is enough to see "what's under this product"
// without scrolling a separate Dependencies tab.
export function DAGTab({ productId }: { productId: string }) {
  const hierarchy = useProductHierarchy(productId)

  if (hierarchy.isLoading) {
    return (
      <Card>
        <CardContent className='space-y-1.5 p-3'>
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className='h-6 w-full' />
          ))}
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
    <Card>
      <CardContent className='p-3'>
        <TreeRow node={hierarchy.data} depth={0} rootId={productId} />
      </CardContent>
    </Card>
  )
}

function TreeRow({
  node,
  depth,
  rootId,
}: {
  node: HierarchyNode
  depth: number
  rootId: string
}) {
  const children = [
    ...(node.sub_products ?? []),
    ...(node.children ?? []),
  ]
  const isRoot = node.id === rootId

  return (
    <div>
      <div
        className='flex items-center gap-2 py-0.5 text-sm'
        style={{ paddingInlineStart: `${depth * 16}px` }}
      >
        <span className='text-muted-foreground font-mono text-xs'>
          {depth === 0 ? '●' : '└'}
        </span>
        {isRoot ? (
          <span className='font-medium'>{node.title}</span>
        ) : (
          <Link
            to='/products'
            search={{ id: node.id, tab: 'description' }}
            className='hover:underline'
          >
            {node.title}
          </Link>
        )}
        <Badge variant='outline' className='text-[9px] uppercase'>
          {node.type}
        </Badge>
        <Badge
          variant='secondary'
          className={`text-[9px] uppercase ${STATUS_COLOR[node.status] ?? 'bg-zinc-500/15'}`}
        >
          {node.status}
        </Badge>
      </div>
      {children.map((c) => (
        <TreeRow key={c.id} node={c} depth={depth + 1} rootId={rootId} />
      ))}
    </div>
  )
}
